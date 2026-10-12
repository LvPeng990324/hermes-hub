package server

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"hermeshub/internal/config"
	"hermeshub/internal/models"

	"gorm.io/gorm"
)

// 用户提及关键字（@用户 / @user 等）
var userAliases = []string{"用户", "user", "admin", "管理员", "主人"}

// Orchestrator 消息总线与指令编排：解析 @、组装上下文、派发指令
type Orchestrator struct {
	db  *gorm.DB
	cfg *config.Config
	hub *Hub
}

func NewOrchestrator(db *gorm.DB, cfg *config.Config, hub *Hub) *Orchestrator {
	return &Orchestrator{db: db, cfg: cfg, hub: hub}
}

// UserSendMessage 用户在项目中发送消息，并派发给被 @ 的实例
func (o *Orchestrator) UserSendMessage(projectID uint, content string) (*models.Message, error) {
	members, err := o.membersOf(projectID)
	if err != nil {
		return nil, err
	}
	mentionIDs, mentionUser := parseMentions(content, members)
	idsJSON, _ := json.Marshal(mentionIDs)

	msg := &models.Message{
		ProjectID:  projectID,
		SenderType: models.SenderUser,
		SenderName: "用户",
		Content:    content,
		MsgType:    models.MsgTypeNormal,
		MentionIDs: string(idsJSON),
		MentionUser: mentionUser,
	}
	if err := o.db.Create(msg).Error; err != nil {
		return nil, err
	}
	o.dispatchToMentioned(projectID, msg, members, mentionIDs, 0)
	return msg, nil
}

// AgentSendMessage Hermes 通过工具 API 发消息；若 @ 了其他实例则继续路由
func (o *Orchestrator) AgentSendMessage(member *models.ProjectMember, instanceName, content, msgType string, extraMentions []string, mentionUser bool) (*models.Message, error) {
	if msgType != models.MsgTypeQuestion {
		msgType = models.MsgTypeNormal
	}
	members, err := o.membersOf(member.ProjectID)
	if err != nil {
		return nil, err
	}
	mentionIDs, parsedUser := parseMentions(content, members)
	mentionUser = mentionUser || parsedUser

	// 合并显式指定的提及名单
	for _, name := range extraMentions {
		for _, m := range members {
			if strings.EqualFold(m.Instance.Name, name) && !containsUint(mentionIDs, m.InstanceID) {
				mentionIDs = append(mentionIDs, m.InstanceID)
			}
		}
	}
	idsJSON, _ := json.Marshal(mentionIDs)

	msg := &models.Message{
		ProjectID:        member.ProjectID,
		SenderType:       models.SenderAgent,
		SenderInstanceID: &member.InstanceID,
		SenderName:       instanceName,
		Content:          content,
		MsgType:          msgType,
		MentionIDs:       string(idsJSON),
		MentionUser:      mentionUser,
	}
	if err := o.db.Create(msg).Error; err != nil {
		return nil, err
	}

	// agent 之间互相 @ 时继续路由；排除发送者自己，带风暴保护
	var targets []uint
	for _, id := range mentionIDs {
		if id != member.InstanceID {
			targets = append(targets, id)
		}
	}
	o.dispatchToMentioned(member.ProjectID, msg, members, targets, member.InstanceID)
	return msg, nil
}

// dispatchToMentioned 向被 @ 的成员逐一创建派发任务并推送
// excludeInstanceID 用于 agent 触发场景排除发送者；风暴保护防止 agent 间无限循环
func (o *Orchestrator) dispatchToMentioned(projectID uint, trigger *models.Message, members []models.ProjectMember, mentionIDs []uint, excludeInstanceID uint) {
	if len(mentionIDs) == 0 {
		return
	}
	// 风暴保护：10 分钟内项目派发超过 40 次则拒绝新增
	var recent int64
	o.db.Model(&models.Dispatch{}).Where("project_id = ? AND created_at > ?", projectID, time.Now().Add(-10*time.Minute)).Count(&recent)
	if recent >= 40 {
		o.postSystemMessage(projectID, "⚠️ 派发过于频繁（10 分钟内超过 40 次），已暂停自动派发，请检查 agent 是否陷入循环")
		return
	}

	for _, m := range members {
		if !containsUint(mentionIDs, m.InstanceID) || m.InstanceID == excludeInstanceID {
			continue
		}
		o.createAndDispatch(&m, trigger)
	}
}

// createAndDispatch 创建派发记录、组装指令上下文并推送连接器
func (o *Orchestrator) createAndDispatch(member *models.ProjectMember, trigger *models.Message) {
	d := &models.Dispatch{
		ProjectID:    member.ProjectID,
		InstanceID:   member.InstanceID,
		InstanceName: member.Instance.Name,
		MessageID:    trigger.ID,
		Status:       models.DispatchPending,
	}
	if err := o.db.Create(d).Error; err != nil {
		log.Printf("[orch] 创建派发失败: %v", err)
		return
	}

	instruction, err := o.BuildInstruction(member, trigger)
	if err != nil {
		o.finishDispatch(d, models.DispatchFailed, "", "组装指令上下文失败: "+err.Error())
		return
	}

	if err := o.hub.SendDispatch(member.InstanceID, d.ID, instruction, member.ToolToken); err != nil {
		o.finishDispatch(d, models.DispatchFailed, "", "实例离线或连接异常")
		o.postSystemMessage(member.ProjectID, fmt.Sprintf("📡 向 %s 派发指令失败：实例离线（消息 #%d）", member.Instance.Name, trigger.ID))
		return
	}
	log.Printf("[orch] 已向实例 %s 派发指令 (dispatch=%d, project=%d)", member.Instance.Name, d.ID, member.ProjectID)
}

// OnAck 连接器确认收到指令
func (o *Orchestrator) OnAck(dispatchID uint) {
	o.db.Model(&models.Dispatch{}).Where("id = ? AND status = ?", dispatchID, models.DispatchPending).
		Updates(map[string]any{"status": models.DispatchRunning})
}

// OnResult 连接器回传执行结果
func (o *Orchestrator) OnResult(dispatchID uint, status, output, errMsg string) {
	var d models.Dispatch
	if err := o.db.First(&d, dispatchID).Error; err != nil {
		return
	}
	if status != models.DispatchDone {
		status = models.DispatchFailed
	}
	o.finishDispatch(&d, status, output, errMsg)

	// 若 Hermes 执行期间没有通过工具 API 发言，则把返回文本代为发布，避免用户看不到结果
	if status == models.DispatchDone && strings.TrimSpace(output) != "" {
		var cnt int64
		o.db.Model(&models.Message{}).
			Where("project_id = ? AND sender_instance_id = ? AND created_at >= ?", d.ProjectID, d.InstanceID, d.CreatedAt).
			Count(&cnt)
		if cnt == 0 {
			var inst models.Instance
			if err := o.db.First(&inst, d.InstanceID).Error; err == nil {
				msg := &models.Message{
					ProjectID:        d.ProjectID,
					SenderType:       models.SenderAgent,
					SenderInstanceID: &d.InstanceID,
					SenderName:       inst.Name,
					Content:          output,
					MsgType:          models.MsgTypeNormal,
					MentionIDs:       "[]",
				}
				o.db.Create(msg)
			}
		}
	}
	if status == models.DispatchFailed && errMsg != "" {
		o.postSystemMessage(d.ProjectID, fmt.Sprintf("❌ 实例 %s 执行指令失败：%s", d.InstanceName, errMsg))
	}
}

func (o *Orchestrator) finishDispatch(d *models.Dispatch, status, output, errMsg string) {
	now := time.Now()
	o.db.Model(&models.Dispatch{}).Where("id = ?", d.ID).Updates(map[string]any{
		"status":       status,
		"output":       output,
		"error":        errMsg,
		"completed_at": &now,
	})
}

// TimeoutSweeper 定期把超时未完成的派发标记为 timeout
func (o *Orchestrator) TimeoutSweeper() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		deadline := time.Now().Add(-time.Duration(o.cfg.DispatchTimeoutMinute) * time.Minute)
		o.db.Model(&models.Dispatch{}).
			Where("status IN ? AND created_at < ?", []string{models.DispatchPending, models.DispatchRunning}, deadline).
			Updates(map[string]any{"status": models.DispatchTimeout, "error": "执行超时"})
	}
}

func (o *Orchestrator) postSystemMessage(projectID uint, content string) {
	msg := &models.Message{
		ProjectID:  projectID,
		SenderType: models.SenderSys,
		SenderName: "系统",
		Content:    content,
		MsgType:    models.MsgTypeSystem,
		MentionIDs: "[]",
	}
	o.db.Create(msg)
}

func (o *Orchestrator) membersOf(projectID uint) ([]models.ProjectMember, error) {
	var members []models.ProjectMember
	err := o.db.Preload("Instance").Where("project_id = ?", projectID).Find(&members).Error
	return members, err
}

// parseMentions 从内容中解析 @提及；按实例名长度降序匹配，避免短名误伤
func parseMentions(content string, members []models.ProjectMember) (ids []uint, mentionUser bool) {
	sorted := make([]models.ProjectMember, len(members))
	copy(sorted, members)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].Instance.Name) > len(sorted[j].Instance.Name)
	})
	for _, m := range sorted {
		if m.Instance == nil || m.Instance.Name == "" {
			continue
		}
		if strings.Contains(content, "@"+m.Instance.Name) && !containsUint(ids, m.InstanceID) {
			ids = append(ids, m.InstanceID)
		}
	}
	lower := strings.ToLower(content)
	for _, alias := range userAliases {
		if strings.Contains(lower, "@"+strings.ToLower(alias)) {
			mentionUser = true
			break
		}
	}
	return ids, mentionUser
}

func containsUint(s []uint, v uint) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
