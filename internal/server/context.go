package server

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"hermeshub/internal/config"
	"hermeshub/internal/models"

	"gorm.io/gorm"
)

// BuildInstruction 组装下发给 Hermes 的完整指令上下文
// 内容：平台规则 + 工具 API 文档 + 项目信息 + 成员角色 + 看板 + 状态总结 + 共享文件清单 + 最近消息 + 当前指令
func (o *Orchestrator) BuildInstruction(member *models.ProjectMember, trigger *models.Message) (string, error) {
	var project models.Project
	if err := o.db.First(&project, member.ProjectID).Error; err != nil {
		return "", err
	}
	members, err := o.membersOf(project.ID)
	if err != nil {
		return "", err
	}

	var b strings.Builder

	// 1. 平台规则与身份
	fmt.Fprintf(&b, `# 平台规则
你是 HermesHub 多 agent 协作平台中的一个 Hermes 实例，实例名为「%s」。
你当前在协作项目「%s」中工作，项目中有多个与你一样的 Hermes agent，由平台负责消息路由。
工作规则：
1. 平台已为你提供名为 hermeshub 的 MCP 工具集（发消息、读消息、看板、状态总结、共享文件、拉取上下文），优先直接调用这些工具，不要改用 curl 等 shell 命令。
2. 需要其他 agent 协作时，在消息中用 @实例名 提及对方（send_message 的 mentions 参数），平台会把消息连同最新上下文转发给它。
3. 遇到卡点、需要用户决策或确认时，发送 msg_type=question 的消息并在正文中 @用户，然后停止推进，等待用户答复。
4. 任务有进展时及时更新看板卡片状态；阶段性成果写入共享文件区；重要状态变化更新项目状态总结。
5. 你收到的共享文件路径相对于项目共享目录，请通过文件 API 读写，不要假设本地路径。
6. 完成当前指令后，直接输出简要结果说明即可，详细信息请通过发消息 API 留存在会话中。

`, member.Instance.Name, project.Name)

	// 2. 工具说明（MCP 工具集为主，HTTP 工具 API 为兜底）
	base := strings.TrimRight(o.cfg.PlatformPublicURL, "/")
	fmt.Fprintf(&b, `# 平台工具（MCP 工具集 "hermeshub"）
你的 MCP 客户端中已配置名为 hermeshub 的工具集，包含以下工具，优先使用：
send_message（发消息，支持 mentions @实例名、msg_type=question 向用户提问）、
read_messages（读最近消息）、get_context（拉取完整上下文）、
get_summary / update_summary（状态总结读写）、
list_cards / create_card / update_card / delete_card（看板，status: todo/doing/done）、
list_files / read_file / write_file / delete_file（共享文件，路径相对项目共享目录）。

# HTTP 工具 API（兜底，仅当 MCP 工具不可用时使用）
Base URL: %s
认证方式: 请求头 Authorization: Bearer %s
（该令牌已绑定你的身份与当前项目，请勿泄露，不要用它访问其他项目）

| 方法与路径 | 说明 | 请求体/参数 |
|---|---|---|
| POST /api/tool/messages | 发送消息到项目会话 | {"content":"...", "msg_type":"normal或question", "mentions":["实例名"], "mention_user":false} |
| GET /api/tool/messages?limit=20 | 读取最近消息 | - |
| GET /api/tool/context | 重新拉取完整上下文 | - |
| GET /api/tool/summary | 读取项目状态总结 | - |
| PUT /api/tool/summary | 覆盖更新项目状态总结 | {"content":"..."} |
| GET /api/tool/cards | 查看看板卡片 | - |
| POST /api/tool/cards | 新建看板卡片 | {"title":"...", "description":"...", "status":"todo", "assignee":"实例名"} |
| PATCH /api/tool/cards/{id} | 更新卡片（状态/负责人/内容） | {"status":"todo/doing/done", ...} |
| DELETE /api/tool/cards/{id} | 删除卡片 | - |
| GET /api/tool/files | 递归列出共享文件 | - |
| GET /api/tool/files/{路径} | 下载共享文件 | - |
| PUT /api/tool/files/{路径} | 上传/覆盖共享文件（请求体为文件内容，支持子目录） | 原始字节 |
| DELETE /api/tool/files/{路径} | 删除共享文件 | - |

`, base, member.ToolToken)

	// 3. 项目信息
	fmt.Fprintf(&b, "# 项目信息\n名称: %s\n描述: %s\n目标: %s\n要求: %s\n\n",
		project.Name, emptyDash(project.Description), emptyDash(project.Goal), emptyDash(project.Requirements))

	// 4. 成员与角色
	b.WriteString("# 项目成员与角色\n")
	for _, m := range members {
		self := ""
		if m.ID == member.ID {
			self = "（这是你）"
		}
		fmt.Fprintf(&b, "- %s%s：%s\n", m.Instance.Name, self, emptyDash(m.RoleDescription))
	}
	fmt.Fprintf(&b, "- 用户：项目发起人，通过会话下达指令、回答询问\n\n")

	// 5. 看板快照
	writeKanbanSection(o.db, &b, project.ID)

	// 6. 状态总结
	b.WriteString("# 项目状态总结\n")
	if strings.TrimSpace(project.Summary) == "" {
		b.WriteString("（暂无）\n\n")
	} else {
		b.WriteString(project.Summary + "\n\n")
	}

	// 7. 共享文件清单
	writeFilesSection(&b, o.cfg, project.ID)

	// 8. 最近消息
	writeRecentMessages(o.db, &b, project.ID, o.cfg.RecentMessagesLimit)

	// 9. 当前指令
	fmt.Fprintf(&b, "# 当前指令\n来自「%s」的消息（消息ID=%d，时间=%s）：\n%s\n",
		trigger.SenderName, trigger.ID, trigger.CreatedAt.Format(time.RFC3339), trigger.Content)

	return b.String(), nil
}

func writeKanbanSection(db *gorm.DB, b *strings.Builder, projectID uint) {
	var cards []models.KanbanCard
	db.Where("project_id = ?", projectID).Order("status, sort_order, id").Find(&cards)
	b.WriteString("# 项目看板\n")
	columns := []struct {
		key, title string
	}{
		{models.StatusTodo, "待办"},
		{models.StatusInProgress, "进行中"},
		{models.StatusReview, "待确认"},
		{models.StatusDone, "完成"},
	}
	if len(cards) == 0 {
		b.WriteString("（空）\n\n")
		return
	}
	for _, col := range columns {
		b.WriteString("## " + col.title + "\n")
		for _, c := range cards {
			if c.Status != col.key {
				continue
			}
			assignee := "未指派"
			if c.AssigneeName != "" {
				assignee = c.AssigneeName
			}
			fmt.Fprintf(b, "- [#%d] %s（负责人: %s）%s\n", c.ID, c.Title, assignee, emptyDashParen(c.Description))
		}
	}
	b.WriteString("\n")
}

func writeFilesSection(b *strings.Builder, cfg *config.Config, projectID uint) {
	b.WriteString("# 共享文件清单\n")
	root := projectFilesDir(cfg, projectID)
	var entries []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		entries = append(entries, fmt.Sprintf("- %s（%d 字节）", filepath.ToSlash(rel), info.Size()))
		return nil
	})
	if len(entries) == 0 {
		b.WriteString("（空）\n\n")
		return
	}
	sort.Strings(entries)
	if len(entries) > 100 {
		entries = append(entries[:100], fmt.Sprintf("- ... 共 %d 个文件，仅列出前 100 个，完整列表请调用文件 API", len(entries)))
	}
	for _, e := range entries {
		b.WriteString(e + "\n")
	}
	b.WriteString("\n")
}

func writeRecentMessages(db *gorm.DB, b *strings.Builder, projectID uint, limit int) {
	var msgs []models.Message
	db.Where("project_id = ?", projectID).Order("id DESC").Limit(limit).Find(&msgs)
	// 反转为时间正序
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	b.WriteString("# 最近消息\n")
	if len(msgs) == 0 {
		b.WriteString("（暂无）\n\n")
		return
	}
	for _, m := range msgs {
		tag := ""
		if m.MsgType == models.MsgTypeQuestion {
			tag = "【询问】"
		}
		fmt.Fprintf(b, "[%s] %s%s: %s\n", m.CreatedAt.Format("01-02 15:04"), tag, m.SenderName, m.Content)
	}
	b.WriteString("\n")
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "（无）"
	}
	return s
}

func emptyDashParen(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return " — " + s
}
