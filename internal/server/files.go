package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"hermeshub/internal/config"

	"github.com/gin-gonic/gin"
)

// projectFilesDir 项目共享文件目录：<root>/<project_id>/
func projectFilesDir(cfg *config.Config, projectID uint) string {
	return filepath.Join(cfg.SharedFilesRoot, strconv.Itoa(int(projectID)))
}

// safeJoin 防止路径穿越，rel 必须落在 root 之内
func safeJoin(root, rel string) (string, bool) {
	cleaned := filepath.Clean("/" + rel)           // 归一化并锁在根部
	full := filepath.Join(root, cleaned)
	rootAbs, err1 := filepath.Abs(root)
	fullAbs, err2 := filepath.Abs(full)
	if err1 != nil || err2 != nil {
		return "", false
	}
	if fullAbs != rootAbs && !strings.HasPrefix(fullAbs, rootAbs+string(os.PathSeparator)) {
		return "", false
	}
	return fullAbs, true
}

// fileEntry 文件清单条目
type fileEntry struct {
	Path    string `json:"path"` // 相对路径（/ 分隔）
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

// listProjectFiles 递归列出项目共享文件
func listProjectFiles(root string) ([]fileEntry, error) {
	var out []fileEntry
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return out, nil
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		out = append(out, fileEntry{
			Path:    filepath.ToSlash(rel),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
		})
		return nil
	})
	return out, err
}

// handleListFiles GET 文件清单
func handleListFiles(cfg *config.Config, projectID uint) gin.HandlerFunc {
	return func(c *gin.Context) {
		files, err := listProjectFiles(projectFilesDir(cfg, projectID))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"files": files})
	}
}

// handleDownloadFile GET 下载文件（通配路径）
func handleDownloadFile(cfg *config.Config, projectID uint) gin.HandlerFunc {
	return func(c *gin.Context) {
		root := projectFilesDir(cfg, projectID)
		full, ok := safeJoin(root, c.Param("filepath"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法路径"})
			return
		}
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
			return
		}
		c.FileAttachment(full, info.Name())
	}
}

// handleUploadFileTool 工具 API：PUT 原始字节上传
func handleUploadFileTool(cfg *config.Config, projectID uint) gin.HandlerFunc {
	return func(c *gin.Context) {
		root := projectFilesDir(cfg, projectID)
		full, ok := safeJoin(root, c.Param("filepath"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法路径"})
			return
		}
		data, err := c.GetRawData()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "读取请求体失败"})
			return
		}
		if len(data) > 200<<20 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "文件超过 200MB 限制"})
			return
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "path": filepath.ToSlash(mustRel(root, full))})
	}
}

// handleUploadFileAdmin 管理端：multipart 上传，支持指定子目录
func handleUploadFileAdmin(cfg *config.Config, projectID uint) gin.HandlerFunc {
	return func(c *gin.Context) {
		root := projectFilesDir(cfg, projectID)
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少文件字段 file"})
			return
		}
		if file.Size > 200<<20 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "文件超过 200MB 限制"})
			return
		}
		rel := filepath.Join(c.PostForm("dir"), file.Filename)
		full, ok := safeJoin(root, rel)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法路径"})
			return
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := c.SaveUploadedFile(file, full); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "path": filepath.ToSlash(mustRel(root, full))})
	}
}

// handleDeleteFile DELETE 删除文件
func handleDeleteFile(cfg *config.Config, projectID uint) gin.HandlerFunc {
	return func(c *gin.Context) {
		root := projectFilesDir(cfg, projectID)
		full, ok := safeJoin(root, c.Param("filepath"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法路径"})
			return
		}
		if err := os.Remove(full); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在或删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func mustRel(root, full string) string {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	rel, _ := filepath.Rel(rootAbs, full)
	return rel
}
