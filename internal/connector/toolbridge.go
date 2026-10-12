package connector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// 工具桥：由 hermes acp 会话以 stdio MCP Server 方式拉起，
// 把平台的 /api/tool/* HTTP 工具 API 暴露为类型化 MCP 工具，
// 让 agent 原生调用（JSON over stdio，全程 UTF-8），避免走 shell 产生编码问题。
//
// 环境变量（由连接器在注入 mcpServers 时传入）：
//   HERMESHUB_PLATFORM_URL 平台地址，如 http://192.168.1.10:8080
//   HERMESHUB_TOOL_TOKEN   当前派发的「实例×项目」工具令牌

// RunToolBridge 工具桥入口：hermeshub-connector toolbridge
func RunToolBridge() error {
	base := strings.TrimRight(os.Getenv("HERMESHUB_PLATFORM_URL"), "/")
	token := os.Getenv("HERMESHUB_TOOL_TOKEN")
	if base == "" || token == "" {
		fmt.Fprintln(os.Stderr, "toolbridge: 缺少 HERMESHUB_PLATFORM_URL 或 HERMESHUB_TOOL_TOKEN")
		os.Exit(2)
	}
	b := &toolBridge{
		base: base,
		http: &http.Client{Timeout: 60 * time.Second},
		auth: "Bearer " + token,
	}
	return b.serve(os.Stdin, os.Stdout)
}

type toolBridge struct {
	base string
	http *http.Client
	auth string
}

// bridgeRequest tools/call 转发描述：参数 → HTTP 请求
type bridgeRequest struct {
	method   string
	path     string                   // 可含 :param 占位符，从 args 取值
	body     map[string]any           // JSON 请求体字段（nil 表示无体）；key 以 "?" 结尾表示可选
	raw      string                   // args 中作为原始请求体的字段名（文件上传）
	query    map[string]string        // query 参数：args字段名 → query名
	pathArgs []string                 // 参与 path 占位的 args 字段（校验必填）
}

type bridgeTool struct {
	name        string
	description string
	schema      map[string]any
	req         bridgeRequest
}

// bridgeTools 工具清单：与平台 /api/tool/* 一一对应
func (b *toolBridge) toolList() []bridgeTool {
	obj := func(props map[string]any, required []string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	num := func(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }

	return []bridgeTool{
		{
			name:        "send_message",
			description: "在项目群聊里发消息。mentions 填实例名可 @ 其他成员触发协作；mention_user=true 且 msg_type=question 表示向用户提问并停下等待",
			schema: obj(map[string]any{
				"content":      str("消息内容（支持中文）"),
				"msg_type":     str("消息类型：normal（默认）或 question（向用户提问）"),
				"mentions":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "要 @ 的实例名列表"},
				"mention_user": map[string]any{"type": "boolean", "description": "是否 @ 用户"},
			}, []string{"content"}),
			req: bridgeRequest{method: "POST", path: "/api/tool/messages",
				body: map[string]any{"content": nil, "msg_type?": nil, "mentions": nil, "mention_user": nil}},
		},
		{
			name:        "read_messages",
			description: "拉取项目最近消息（默认 20 条，最多 100）",
			schema:      obj(map[string]any{"limit": num("条数，默认 20")}, nil),
			req:         bridgeRequest{method: "GET", path: "/api/tool/messages", query: map[string]string{"limit": "limit"}},
		},
		{
			name:        "get_context",
			description: "重新拉取完整项目上下文（规则、目标、成员、看板、总结、文件清单等）",
			schema:      obj(map[string]any{}, nil),
			req:         bridgeRequest{method: "GET", path: "/api/tool/context"},
		},
		{
			name:        "get_summary",
			description: "读取项目状态总结",
			schema:      obj(map[string]any{}, nil),
			req:         bridgeRequest{method: "GET", path: "/api/tool/summary"},
		},
		{
			name:        "update_summary",
			description: "更新项目状态总结（覆盖写入）",
			schema:      obj(map[string]any{"content": str("新的总结内容")}, []string{"content"}),
			req:         bridgeRequest{method: "PUT", path: "/api/tool/summary",
				body: map[string]any{"content": nil}},
		},
		{
			name:        "list_cards",
			description: "查看看板卡片",
			schema:      obj(map[string]any{}, nil),
			req:         bridgeRequest{method: "GET", path: "/api/tool/cards"},
		},
		{
			name:        "create_card",
			description: "创建看板卡片。status: todo / doing / done",
			schema: obj(map[string]any{
				"title":       str("卡片标题"),
				"description": str("补充说明"),
				"status":      str("初始状态：todo（默认）/ doing / done"),
				"assignee":    str("负责人实例名"),
			}, []string{"title"}),
			req: bridgeRequest{method: "POST", path: "/api/tool/cards",
				body: map[string]any{"title": nil, "description": nil, "status": nil, "assignee": nil}},
		},
		{
			name:        "update_card",
			description: "更新看板卡片（只传要改的字段）",
			schema: obj(map[string]any{
				"card_id":     num("卡片 ID"),
				"title":       str("新标题"),
				"description": str("新说明"),
				"status":      str("todo / doing / done"),
				"assignee":    str("负责人实例名"),
			}, []string{"card_id"}),
			req: bridgeRequest{method: "PATCH", path: "/api/tool/cards/:card_id",
				pathArgs: []string{"card_id"},
				body:     map[string]any{"title": nil, "description": nil, "status": nil, "assignee": nil}},
		},
		{
			name:        "delete_card",
			description: "删除看板卡片",
			schema:      obj(map[string]any{"card_id": num("卡片 ID")}, []string{"card_id"}),
			req:         bridgeRequest{method: "DELETE", path: "/api/tool/cards/:card_id", pathArgs: []string{"card_id"}},
		},
		{
			name:        "list_files",
			description: "列出项目共享文件",
			schema:      obj(map[string]any{}, nil),
			req:         bridgeRequest{method: "GET", path: "/api/tool/files"},
		},
		{
			name:        "read_file",
			description: "读取项目共享文件内容；文本直接返回，二进制返回 base64",
			schema:      obj(map[string]any{"path": str("文件路径，如 docs/a.txt")}, []string{"path"}),
			req:         bridgeRequest{method: "GET", path: "/api/tool/files/:path",
				pathArgs: []string{"path"}, raw: "@file"},
		},
		{
			name:        "write_file",
			description: "写入项目共享文件（自动创建目录）。content 为文本；写二进制用 content_base64",
			schema: obj(map[string]any{
				"path":            str("文件路径，如 docs/a.txt"),
				"content":         str("文本内容"),
				"content_base64":  str("base64 编码内容（与 content 二选一）"),
			}, []string{"path"}),
			req: bridgeRequest{method: "PUT", path: "/api/tool/files/:path",
				pathArgs: []string{"path"}, raw: "@file"},
		},
		{
			name:        "delete_file",
			description: "删除项目共享文件",
			schema:      obj(map[string]any{"path": str("文件路径")}, []string{"path"}),
			req:         bridgeRequest{method: "DELETE", path: "/api/tool/files/:path", pathArgs: []string{"path"}},
		},
	}
}

// serve stdio 上的 JSON-RPC 主循环（逐行，同步处理）
func (b *toolBridge) serve(stdin io.Reader, stdout io.Writer) error {
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	w := bufio.NewWriter(stdout)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var msg struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(line, &msg); err != nil || msg.Method == "" {
			continue // 无法解析或非请求（响应/通知里 method 为空）
		}
		resp := b.handle(msg.Method, msg.Params)
		if resp == nil {
			continue // 通知，无需响应
		}
		if len(msg.ID) > 0 && string(msg.ID) != "null" {
			out := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(msg.ID)}
			if resp.Err != nil {
				out["error"] = map[string]any{"code": resp.Err.Code, "message": resp.Err.Message}
			} else {
				out["result"] = resp.Result
			}
			data, _ := json.Marshal(out)
			w.Write(append(data, '\n'))
			w.Flush()
		}
	}
	return scanner.Err()
}

type bridgeResp struct {
	Result any
	Err    *acpError
}

func (b *toolBridge) handle(method string, params json.RawMessage) *bridgeResp {
	switch method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(params, &p)
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = "2025-11-25"
		}
		return &bridgeResp{Result: map[string]any{
			"protocolVersion": p.ProtocolVersion, // 回显对方请求的版本，避免协商分歧
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "hermeshub-tools", "version": "1.0.0"},
			"instructions":    "HermesHub 平台工具集：发消息、看板、总结、共享文件，直接调用即可，不要改用 curl。",
		}}
	case "server/discover":
		return &bridgeResp{Result: map[string]any{
			"supportedVersions": []string{"2026-07-28", "2025-11-25", "2025-06-18", "2025-03-26"},
			"capabilities":      map[string]any{"tools": map[string]any{}},
			"serverInfo":        map[string]any{"name": "hermeshub-tools", "version": "1.0.0"},
			"instructions":      "HermesHub 平台工具集：发消息、看板、总结、共享文件，直接调用即可，不要改用 curl。",
		}}
	case "ping":
		return &bridgeResp{Result: map[string]any{}}
	case "tools/list":
		tools := []map[string]any{}
		for _, t := range b.toolList() {
			tools = append(tools, map[string]any{
				"name": t.name, "description": t.description, "inputSchema": t.schema,
			})
		}
		return &bridgeResp{Result: map[string]any{"tools": tools}}
	case "tools/call":
		return b.handleCall(params)
	default:
		// notifications/* 等通知无需响应
		if strings.HasPrefix(method, "notifications/") {
			return nil
		}
		return &bridgeResp{Err: &acpError{Code: -32601, Message: "method not found: " + method}}
	}
}

// handleCall 执行 tools/call：把 MCP 工具调用转发为平台工具 API 请求
func (b *toolBridge) handleCall(params json.RawMessage) *bridgeResp {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
		return &bridgeResp{Err: &acpError{Code: -32602, Message: "invalid tools/call params"}}
	}
	var tool *bridgeTool
	for _, t := range b.toolList() {
		if t.name == p.Name {
			tool = &t
			break
		}
	}
	if tool == nil {
		return &bridgeResp{Err: &acpError{Code: -32602, Message: "未知工具: " + p.Name}}
	}

	var args map[string]any
	if len(p.Arguments) > 0 {
		if err := json.Unmarshal(p.Arguments, &args); err != nil {
			return callError(fmt.Sprintf("参数不是合法 JSON: %v", err))
		}
	}
	for _, key := range tool.req.pathArgs {
		if args[key] == nil || fmt.Sprint(args[key]) == "" {
			return callError("缺少必填参数: " + key)
		}
	}

	// 组装请求
	path := tool.req.path
	for _, key := range tool.req.pathArgs {
		path = strings.ReplaceAll(path, ":"+key, url.PathEscape(fmt.Sprint(args[key])))
		delete(args, key)
	}
	if len(tool.req.query) > 0 {
		qs := url.Values{}
		for argKey, queryName := range tool.req.query {
			if v, ok := args[argKey]; ok && fmt.Sprint(v) != "" {
				qs.Set(queryName, fmt.Sprint(v))
				delete(args, argKey)
			}
		}
		if len(qs) > 0 {
			path += "?" + qs.Encode()
		}
	}

	var bodyReader io.Reader
	if tool.req.raw == "@file" {
		// 文件读写：原始字节
		var raw []byte
		if b64, ok := args["content_base64"].(string); ok && b64 != "" {
			dec, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				return callError("content_base64 解码失败: " + err.Error())
			}
			raw = dec
		} else if content, ok := args["content"].(string); ok {
			raw = []byte(content)
		}
		bodyReader = bytes.NewReader(raw)
	} else if tool.req.body != nil {
		body := map[string]any{}
		for field := range tool.req.body {
			key := strings.TrimSuffix(field, "?")
			if v, ok := args[key]; ok {
				body[key] = v
			}
		}
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(context.Background(), tool.req.method, b.base+path, bodyReader)
	if err != nil {
		return callError("构造请求失败: " + err.Error())
	}
	req.Header.Set("Authorization", b.auth)
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	log.Printf("[toolbridge] %s %s (tool=%s)", tool.req.method, path, tool.name)
	resp, err := b.http.Do(req)
	if err != nil {
		return callError("请求平台失败: " + err.Error())
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))

	if tool.req.raw == "@file" && tool.req.method == "GET" && resp.StatusCode == http.StatusOK {
		// 文件下载：文本直接给，二进制 base64
		if utf8.Valid(respBody) {
			return callOK(string(respBody))
		}
		return callOK("（二进制文件，base64 编码）\n" + base64.StdEncoding.EncodeToString(respBody))
	}
	if resp.StatusCode >= 400 {
		return callError(fmt.Sprintf("平台返回 %d: %s", resp.StatusCode, truncate(string(respBody), 500)))
	}
	return callOK(prettyJSON(respBody))
}

func callOK(text string) *bridgeResp {
	return &bridgeResp{Result: map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
	}}
}

func callError(msg string) *bridgeResp {
	return &bridgeResp{Result: map[string]any{
		"content": []map[string]any{{"type": "text", "text": "错误: " + msg}},
		"isError": true,
	}}
}

func prettyJSON(raw []byte) string {
	var v any
	if json.Unmarshal(raw, &v) == nil {
		if out, err := json.MarshalIndent(v, "", "  "); err == nil {
			return string(out)
		}
	}
	return string(raw)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
