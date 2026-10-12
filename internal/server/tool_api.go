package server

import (
	"net/http"
	"strconv"
	"strings"

	"hermeshub/internal/models"

	"github.com/gin-gonic/gin"
)

// toolAuthMiddleware 工具 API 鉴权：Bearer ToolToken → 解析出 成员 + 实例
func (s *Server) toolAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "缺少 Bearer 令牌"})
			return
		}
		token := strings.TrimPrefix(h, "Bearer ")
		var m models.ProjectMember
		if err := s.db.Preload("Instance").Where("tool_token = ?", token).First(&m).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "无效的工具令牌"})
			return
		}
		c.Set("member", &m)
		c.Next()
	}
}

func ctxMember(c *gin.Context) *models.ProjectMember {
	v, _ := c.Get("member")
	return v.(*models.ProjectMember)
}

// handleToolSendMessage Hermes 发消息；@其他实例 会触发路由，@用户 + question 表示询问
func (s *Server) handleToolSendMessage(c *gin.Context) {
	m := ctxMember(c)
	var req struct {
		Content     string   `json:"content" binding:"required"`
		MsgType     string   `json:"msg_type"`
		Mentions    []string `json:"mentions"`
		MentionUser bool     `json:"mention_user"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content 不能为空"})
		return
	}
	msg, err := s.orch.AgentSendMessage(m, m.Instance.Name, req.Content, req.MsgType, req.Mentions, req.MentionUser)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": msg})
}

// handleToolListMessages Hermes 拉取最近消息
func (s *Server) handleToolListMessages(c *gin.Context) {
	m := ctxMember(c)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var msgs []models.Message
	s.db.Where("project_id = ?", m.ProjectID).Order("id DESC").Limit(limit).Find(&msgs)
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	c.JSON(http.StatusOK, gin.H{"messages": msgs})
}

// handleToolContext Hermes 重新拉取完整上下文（以一条虚拟触发消息组装）
func (s *Server) handleToolContext(c *gin.Context) {
	m := ctxMember(c)
	trigger := &models.Message{
		ProjectID:  m.ProjectID,
		SenderType: models.SenderAgent,
		SenderName: m.Instance.Name,
		Content:    "（主动拉取上下文）",
	}
	text, err := s.orch.BuildInstruction(m, trigger)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"context": text})
}

// handleToolGetSummary / handleToolPutSummary
func (s *Server) handleToolGetSummary(c *gin.Context) {
	m := ctxMember(c)
	var p models.Project
	if err := s.db.Select("id", "summary").First(&p, m.ProjectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "项目不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"summary": p.Summary})
}

func (s *Server) handleToolPutSummary(c *gin.Context) {
	m := ctxMember(c)
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := s.db.Model(&models.Project{}).Where("id = ?", m.ProjectID).Update("summary", req.Content).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- 工具 API：看板 ----------

func (s *Server) handleToolListCards(c *gin.Context) {
	m := ctxMember(c)
	var cards []models.KanbanCard
	s.db.Where("project_id = ?", m.ProjectID).Order("status, sort_order, id").Find(&cards)
	c.JSON(http.StatusOK, gin.H{"cards": cards})
}

func (s *Server) handleToolCreateCard(c *gin.Context) {
	m := ctxMember(c)
	var req cardReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title 必填"})
		return
	}
	if !validStatus(req.Status) {
		req.Status = models.StatusTodo
	}
	assigneeID, assigneeName := s.resolveAssignee(m.ProjectID, req.Assignee)
	card := models.KanbanCard{
		ProjectID: m.ProjectID, Title: req.Title, Description: req.Description,
		Status: req.Status, AssigneeInstanceID: assigneeID, AssigneeName: assigneeName,
		CreatedBy: m.Instance.Name,
	}
	if err := s.db.Create(&card).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"card": card})
}

func (s *Server) handleToolUpdateCard(c *gin.Context) {
	m := ctxMember(c)
	cid, _ := strconv.Atoi(c.Param("cid"))
	var req cardReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	updates := map[string]any{}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if validStatus(req.Status) {
		updates["status"] = req.Status
	}
	if req.Assignee != "" {
		assigneeID, assigneeName := s.resolveAssignee(m.ProjectID, req.Assignee)
		updates["assignee_instance_id"] = assigneeID
		updates["assignee_name"] = assigneeName
	}
	res := s.db.Model(&models.KanbanCard{}).Where("id = ? AND project_id = ?", cid, m.ProjectID).Updates(updates)
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "卡片不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleToolDeleteCard(c *gin.Context) {
	m := ctxMember(c)
	cid, _ := strconv.Atoi(c.Param("cid"))
	s.db.Where("id = ? AND project_id = ?", cid, m.ProjectID).Delete(&models.KanbanCard{})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- 工具 API：文件（委托给 files.go 中的处理器） ----------

func (s *Server) toolProjectID(c *gin.Context) uint { return ctxMember(c).ProjectID }
