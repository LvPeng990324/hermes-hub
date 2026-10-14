package server

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	"hermeshub/internal/auth"
	"hermeshub/internal/config"
	"hermeshub/internal/database"
	"hermeshub/web"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Server 平台服务端聚合
type Server struct {
	cfg  *config.Config
	db   *gorm.DB
	hub  *Hub
	orch *Orchestrator
}

// Run 启动平台服务：hermeshub-server [--env .env]
func Run(args []string) error {
	flagSet := flag.NewFlagSet("serve", flag.ContinueOnError)
	envFile := flagSet.String("env", "", ".env 文件路径")
	if err := flagSet.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*envFile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.SharedFilesRoot, 0o755); err != nil {
		return fmt.Errorf("创建共享文件根目录失败: %w", err)
	}

	db, err := database.Init(cfg.DBPath)
	if err != nil {
		return err
	}

	s := &Server{cfg: cfg, db: db}
	s.hub = NewHub(db)
	s.orch = NewOrchestrator(db, cfg, s.hub)
	s.hub.orch = s.orch
	go s.orch.TimeoutSweeper()

	engine := s.setupRouter()
	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("🚀 HermesHub 平台已启动: http://localhost%s", addr)
	log.Printf("   共享文件根目录: %s", cfg.SharedFilesRoot)
	return engine.Run(addr)
}

func (s *Server) setupRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// 健康检查
	r.GET("/api/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	// 登录（无鉴权）
	r.POST("/api/login", s.handleLogin)

	// 连接器 WebSocket 接入（token 鉴权在 hub 内完成）
	r.GET("/ws/connector", s.hub.HandleConnectorWS)

	// 管理端 API（JWT）
	admin := r.Group("/api", auth.AdminMiddleware(s.cfg.JWTSecret))
	{
		admin.GET("/instances", s.handleListInstances)
		admin.POST("/instances", s.handleCreateInstance)
		admin.PATCH("/instances/:id", s.handleUpdateInstance)
		admin.POST("/instances/:id/test", s.handleTestInstance)
		admin.DELETE("/instances/:id", s.handleDeleteInstance)

		admin.GET("/projects", s.handleListProjects)
		admin.POST("/projects", s.handleCreateProject)
		admin.GET("/projects/:id", s.handleGetProject)
		admin.PATCH("/projects/:id", s.handleUpdateProject)
		admin.DELETE("/projects/:id", s.handleDeleteProject)

		admin.GET("/projects/:id/members", s.handleListMembers)
		admin.POST("/projects/:id/members", s.handleAddMember)
		admin.PATCH("/projects/:id/members/:mid", s.handleUpdateMember)
		admin.DELETE("/projects/:id/members/:mid", s.handleRemoveMember)

		admin.GET("/projects/:id/messages", s.handleListMessages)
		admin.POST("/projects/:id/messages", s.handleSendMessage)

		admin.GET("/projects/:id/cards", s.handleListCards)
		admin.POST("/projects/:id/cards", s.handleCreateCard)
		admin.PATCH("/projects/:id/cards/:cid", s.handleUpdateCard)
		admin.DELETE("/projects/:id/cards/:cid", s.handleDeleteCard)

		admin.GET("/projects/:id/summary", s.handleGetSummary)
		admin.PUT("/projects/:id/summary", s.handlePutSummary)

		admin.GET("/projects/:id/dispatches", s.handleListDispatches)

		admin.GET("/projects/:id/files", s.wrapFiles(handleListFiles))
		admin.GET("/projects/:id/files/*filepath", s.wrapFiles(handleDownloadFile))
		admin.POST("/projects/:id/files/upload", s.wrapFiles(handleUploadFileAdmin))
		admin.DELETE("/projects/:id/files/*filepath", s.wrapFiles(handleDeleteFile))
	}

	// 工具 API（Hermes 使用，ToolToken 鉴权，令牌已绑定项目）
	tool := r.Group("/api/tool", s.toolAuthMiddleware())
	{
		tool.POST("/messages", s.handleToolSendMessage)
		tool.GET("/messages", s.handleToolListMessages)
		tool.GET("/context", s.handleToolContext)
		tool.GET("/summary", s.handleToolGetSummary)
		tool.PUT("/summary", s.handleToolPutSummary)
		tool.GET("/cards", s.handleToolListCards)
		tool.POST("/cards", s.handleToolCreateCard)
		tool.PATCH("/cards/:cid", s.handleToolUpdateCard)
		tool.DELETE("/cards/:cid", s.handleToolDeleteCard)

		tool.GET("/files", func(c *gin.Context) { handleListFiles(s.cfg, s.toolProjectID(c))(c) })
		tool.GET("/files/*filepath", func(c *gin.Context) { handleDownloadFile(s.cfg, s.toolProjectID(c))(c) })
		tool.PUT("/files/*filepath", func(c *gin.Context) { handleUploadFileTool(s.cfg, s.toolProjectID(c))(c) })
		tool.DELETE("/files/*filepath", func(c *gin.Context) { handleDeleteFile(s.cfg, s.toolProjectID(c))(c) })
	}

	// 静态资源与 SPA 回退
	s.mountFrontend(r)
	return r
}

// wrapFiles 把 files.go 中的通用处理器绑定到路由参数 :id 对应的共享目录
func (s *Server) wrapFiles(h func(cfg *config.Config, projectID uint) gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		var pid uint
		fmt.Sscanf(c.Param("id"), "%d", &pid)
		if pid == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "项目 ID 非法"})
			return
		}
		h(s.cfg, pid)(c)
	}
}

// mountFrontend 服务内嵌的前端构建产物，非 API 路径回退到 index.html
func (s *Server) mountFrontend(r *gin.Engine) {
	dist, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		log.Printf("[warn] 前端产物未嵌入，仅提供 API: %v", err)
		return
	}
	fileServer := http.FileServer(http.FS(dist))

	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api") || strings.HasPrefix(p, "/ws") {
			c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
			return
		}
		// 静态文件存在则直接服务，否则回退 index.html（SPA 路由）
		// gin 的 NoRoute 会预置 404 状态，这里先重置为 200，否则浏览器会拒绝执行 404 状态下的 JS/CSS
		c.Status(http.StatusOK)
		clean := strings.TrimPrefix(path.Clean(p), "/")
		if clean != "." && clean != "" {
			if f, err := dist.Open(clean); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}
		data, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			c.String(http.StatusNotFound, "前端产物缺失，请先构建 web/")
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		_, _ = c.Writer.Write(data)
	})
}
