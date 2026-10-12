package server

import (
	"net/http"
	"strconv"

	"hermeshub/internal/auth"
	"hermeshub/internal/models"

	"github.com/gin-gonic/gin"
)

// ---------- 登录 ----------

func (s *Server) handleLogin(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入密码"})
		return
	}
	if req.Password != s.cfg.AdminPassword {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "密码错误"})
		return
	}
	token, err := auth.IssueAdminJWT(s.cfg.JWTSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "签发令牌失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}

// ---------- 实例管理 ----------

func (s *Server) handleListInstances(c *gin.Context) {
	var list []models.Instance
	if err := s.db.Order("id DESC").Find(&list).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	online := s.hub.OnlineIDs()
	type instVO struct {
		models.Instance
		Online bool `json:"online"`
	}
	out := make([]instVO, 0, len(list))
	for _, inst := range list {
		out = append(out, instVO{inst, online[inst.ID]})
	}
	c.JSON(http.StatusOK, gin.H{"instances": out})
}

func (s *Server) handleCreateInstance(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称必填"})
		return
	}
	inst := models.Instance{Name: req.Name, Description: req.Description, Token: auth.NewToken()}
	if err := s.db.Create(&inst).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "实例名已存在或数据异常"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"instance": inst})
}

func (s *Server) handleUpdateInstance(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if err := s.db.Model(&models.Instance{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleDeleteInstance(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	// 级联删除成员关系；消息保留作为历史
	s.db.Where("instance_id = ?", id).Delete(&models.ProjectMember{})
	if err := s.db.Delete(&models.Instance{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- 项目管理 ----------

func (s *Server) handleListProjects(c *gin.Context) {
	var list []models.Project
	if err := s.db.Order("id DESC").Find(&list).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	type projVO struct {
		models.Project
		MemberCount  int64 `json:"member_count"`
		MessageCount int64 `json:"message_count"`
	}
	out := make([]projVO, 0, len(list))
	for _, p := range list {
		var mc, gc int64
		s.db.Model(&models.ProjectMember{}).Where("project_id = ?", p.ID).Count(&mc)
		s.db.Model(&models.Message{}).Where("project_id = ?", p.ID).Count(&gc)
		out = append(out, projVO{p, mc, gc})
	}
	c.JSON(http.StatusOK, gin.H{"projects": out})
}

func (s *Server) handleGetProject(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var p models.Project
	if err := s.db.First(&p, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "项目不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"project": p})
}

type projectReq struct {
	Name         string `json:"name" binding:"required"`
	Description  string `json:"description"`
	Goal         string `json:"goal"`
	Requirements string `json:"requirements"`
}

func (s *Server) handleCreateProject(c *gin.Context) {
	var req projectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "项目名必填"})
		return
	}
	p := models.Project{Name: req.Name, Description: req.Description, Goal: req.Goal, Requirements: req.Requirements}
	if err := s.db.Create(&p).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"project": p})
}

func (s *Server) handleUpdateProject(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req projectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "项目名必填"})
		return
	}
	updates := map[string]any{
		"name": req.Name, "description": req.Description,
		"goal": req.Goal, "requirements": req.Requirements,
	}
	if err := s.db.Model(&models.Project{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleDeleteProject(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	s.db.Where("project_id = ?", id).Delete(&models.ProjectMember{})
	s.db.Where("project_id = ?", id).Delete(&models.Message{})
	s.db.Where("project_id = ?", id).Delete(&models.KanbanCard{})
	s.db.Where("project_id = ?", id).Delete(&models.Dispatch{})
	if err := s.db.Delete(&models.Project{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- 项目成员 ----------

func (s *Server) handleListMembers(c *gin.Context) {
	pid, _ := strconv.Atoi(c.Param("id"))
	var members []models.ProjectMember
	if err := s.db.Preload("Instance").Where("project_id = ?", pid).Find(&members).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	online := s.hub.OnlineIDs()
	type memberVO struct {
		models.ProjectMember
		Online bool `json:"online"`
	}
	out := make([]memberVO, 0, len(members))
	for _, m := range members {
		m.ToolToken = "" // 管理端列表不回显工具令牌
		out = append(out, memberVO{m, online[m.InstanceID]})
	}
	c.JSON(http.StatusOK, gin.H{"members": out})
}

func (s *Server) handleAddMember(c *gin.Context) {
	pid, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		InstanceID      uint   `json:"instance_id" binding:"required"`
		RoleDescription string `json:"role_description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择实例"})
		return
	}
	var inst models.Instance
	if err := s.db.First(&inst, req.InstanceID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "实例不存在"})
		return
	}
	m := models.ProjectMember{
		ProjectID: uint(pid), InstanceID: req.InstanceID,
		RoleDescription: req.RoleDescription, ToolToken: auth.NewToken(),
	}
	if err := s.db.Create(&m).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "该实例已在项目中"})
		return
	}
	s.orch.postSystemMessage(uint(pid), "👋 实例 "+inst.Name+" 已加入项目")
	c.JSON(http.StatusOK, gin.H{"member": m})
}

func (s *Server) handleUpdateMember(c *gin.Context) {
	mid, _ := strconv.Atoi(c.Param("mid"))
	var req struct {
		RoleDescription string `json:"role_description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := s.db.Model(&models.ProjectMember{}).Where("id = ?", mid).
		Update("role_description", req.RoleDescription).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleRemoveMember(c *gin.Context) {
	pid, _ := strconv.Atoi(c.Param("id"))
	mid, _ := strconv.Atoi(c.Param("mid"))
	var m models.ProjectMember
	if err := s.db.Preload("Instance").First(&m, mid).Error; err == nil && m.Instance != nil {
		s.orch.postSystemMessage(uint(pid), "👋 实例 "+m.Instance.Name+" 已退出项目")
	}
	s.db.Delete(&models.ProjectMember{}, mid)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- 会话消息 ----------

func (s *Server) handleListMessages(c *gin.Context) {
	pid, _ := strconv.Atoi(c.Param("id"))
	afterID, _ := strconv.Atoi(c.Query("after_id"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := s.db.Where("project_id = ?", pid)
	if afterID > 0 {
		q = q.Where("id > ?", afterID).Order("id ASC").Limit(limit)
	} else {
		q = q.Order("id DESC").Limit(limit)
	}
	var msgs []models.Message
	if err := q.Find(&msgs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 翻转为时间正序
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	c.JSON(http.StatusOK, gin.H{"messages": msgs})
}

func (s *Server) handleSendMessage(c *gin.Context) {
	pid, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "消息内容不能为空"})
		return
	}
	var p models.Project
	if err := s.db.First(&p, pid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "项目不存在"})
		return
	}
	msg, err := s.orch.UserSendMessage(uint(pid), req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": msg})
}

// ---------- 看板 ----------

func (s *Server) handleListCards(c *gin.Context) {
	pid, _ := strconv.Atoi(c.Param("id"))
	var cards []models.KanbanCard
	if err := s.db.Where("project_id = ?", pid).Order("status, sort_order, id").Find(&cards).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cards": cards})
}

type cardReq struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Assignee    string `json:"assignee"` // 负责人实例名
}

func (s *Server) resolveAssignee(projectID uint, name string) (*uint, string) {
	if name == "" {
		return nil, ""
	}
	var m models.ProjectMember
	if err := s.db.Preload("Instance").Where("project_id = ?", projectID).Find(&m, "instance_id IN (?)",
		s.db.Model(&models.Instance{}).Select("id").Where("name = ?", name)).Error; err == nil && m.Instance != nil {
		return &m.InstanceID, m.Instance.Name
	}
	return nil, name
}

func validStatus(st string) bool {
	switch st {
	case models.StatusTodo, models.StatusInProgress, models.StatusReview, models.StatusDone:
		return true
	}
	return false
}

func (s *Server) handleCreateCard(c *gin.Context) {
	pid, _ := strconv.Atoi(c.Param("id"))
	var req cardReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标题必填"})
		return
	}
	if !validStatus(req.Status) {
		req.Status = models.StatusTodo
	}
	assigneeID, assigneeName := s.resolveAssignee(uint(pid), req.Assignee)
	card := models.KanbanCard{
		ProjectID: uint(pid), Title: req.Title, Description: req.Description,
		Status: req.Status, AssigneeInstanceID: assigneeID, AssigneeName: assigneeName,
		CreatedBy: "user",
	}
	if err := s.db.Create(&card).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"card": card})
}

func (s *Server) handleUpdateCard(c *gin.Context) {
	pid, _ := strconv.Atoi(c.Param("id"))
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
		assigneeID, assigneeName := s.resolveAssignee(uint(pid), req.Assignee)
		updates["assignee_instance_id"] = assigneeID
		updates["assignee_name"] = assigneeName
	}
	if err := s.db.Model(&models.KanbanCard{}).Where("id = ? AND project_id = ?", cid, pid).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleDeleteCard(c *gin.Context) {
	cid, _ := strconv.Atoi(c.Param("cid"))
	if err := s.db.Delete(&models.KanbanCard{}, cid).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- 状态总结 ----------

func (s *Server) handleGetSummary(c *gin.Context) {
	pid, _ := strconv.Atoi(c.Param("id"))
	var p models.Project
	if err := s.db.Select("id", "summary", "updated_at").First(&p, pid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "项目不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"summary": p.Summary, "updated_at": p.UpdatedAt})
}

func (s *Server) handlePutSummary(c *gin.Context) {
	pid, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if err := s.db.Model(&models.Project{}).Where("id = ?", pid).Update("summary", req.Content).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- 派发记录 ----------

func (s *Server) handleListDispatches(c *gin.Context) {
	pid, _ := strconv.Atoi(c.Param("id"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	var list []models.Dispatch
	if err := s.db.Where("project_id = ?", pid).Order("id DESC").Limit(limit).Find(&list).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"dispatches": list})
}
