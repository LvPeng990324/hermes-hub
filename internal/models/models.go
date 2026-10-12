package models

import "time"

// Instance 一个已注册的 Hermes 实例
type Instance struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:64;uniqueIndex;not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	Token       string    `gorm:"size:64;uniqueIndex;not null" json:"token"` // 连接器接入令牌
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Project 一个多 agent 协作项目
type Project struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"size:128;not null" json:"name"`
	Description  string    `gorm:"type:text" json:"description"`
	Goal         string    `gorm:"type:text" json:"goal"`
	Requirements string    `gorm:"type:text" json:"requirements"`
	Summary      string    `gorm:"type:text" json:"summary"` // 项目状态总结，由 Hermes 按需更新
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ProjectMember 项目成员：某个 Hermes 实例及其在本项目中的角色描述
type ProjectMember struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	ProjectID       uint      `gorm:"uniqueIndex:idx_project_instance;not null" json:"project_id"`
	InstanceID      uint      `gorm:"uniqueIndex:idx_project_instance;not null" json:"instance_id"`
	RoleDescription string    `gorm:"type:text" json:"role_description"` // 该实例在本项目中的角色，塞入指令上下文
	ToolToken       string    `gorm:"size:64;uniqueIndex;not null" json:"tool_token"` // 工具 API 令牌（实例×项目 粒度）
	CreatedAt       time.Time `json:"created_at"`

	Instance *Instance `gorm:"foreignKey:InstanceID" json:"instance,omitempty"`
}

// 消息类型
const (
	MsgTypeNormal   = "normal"   // 普通消息 / 指令
	MsgTypeQuestion = "question" // agent 向用户的询问（卡点待确认）
	MsgTypeSystem   = "system"   // 系统提示（派发失败、成员加入等）
)

// 发送者类型
const (
	SenderUser  = "user"
	SenderAgent = "agent"
	SenderSys   = "system"
)

// Message 项目会话流中的一条消息
type Message struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ProjectID        uint      `gorm:"index:idx_project_msg;not null" json:"project_id"`
	SenderType       string    `gorm:"size:16;not null" json:"sender_type"` // user / agent / system
	SenderInstanceID *uint     `json:"sender_instance_id,omitempty"`
	SenderName       string    `gorm:"size:64" json:"sender_name"` // 冗余存储，便于展示
	Content          string    `gorm:"type:text" json:"content"`
	MsgType          string    `gorm:"size:16;default:normal" json:"msg_type"`
	MentionIDs       string    `gorm:"size:256" json:"mention_ids"`  // JSON 数组，被 @ 的实例 ID
	MentionUser      bool      `json:"mention_user"`                 // 是否 @ 了用户（询问场景）
	CreatedAt        time.Time `gorm:"index:idx_project_msg" json:"created_at"`
}

// 看板状态列
const (
	StatusTodo       = "todo"        // 待办
	StatusInProgress = "in_progress" // 进行中
	StatusReview     = "review"      // 待确认
	StatusDone       = "done"        // 完成
)

// KanbanCard 项目看板卡片（一个拆解出的子任务）
type KanbanCard struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	ProjectID          uint      `gorm:"index;not null" json:"project_id"`
	Title              string    `gorm:"size:256;not null" json:"title"`
	Description        string    `gorm:"type:text" json:"description"`
	Status             string    `gorm:"size:16;index;default:todo" json:"status"`
	AssigneeInstanceID *uint     `json:"assignee_instance_id,omitempty"`
	AssigneeName       string    `gorm:"size:64" json:"assignee_name"` // 冗余负责人名称
	SortOrder          float64   `gorm:"default:0" json:"sort_order"`
	CreatedBy          string    `gorm:"size:64" json:"created_by"` // 创建者（user 或实例名）
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// 派发状态
const (
	DispatchPending = "pending" // 已创建，等待连接器确认
	DispatchRunning = "running" // 连接器已确认，Hermes 执行中
	DispatchDone    = "done"
	DispatchFailed  = "failed"
	DispatchTimeout = "timeout"
)

// Dispatch 一次指令派发记录（平台 → 连接器 → Hermes）
type Dispatch struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	ProjectID   uint       `gorm:"index;not null" json:"project_id"`
	InstanceID  uint       `gorm:"index;not null" json:"instance_id"`
	InstanceName string    `gorm:"size:64" json:"instance_name"`
	MessageID   uint       `json:"message_id"` // 触发消息
	Status      string     `gorm:"size:16;default:pending" json:"status"`
	Error       string     `gorm:"type:text" json:"error,omitempty"`
	Output      string     `gorm:"type:text" json:"output,omitempty"` // Hermes 最终返回文本
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}
