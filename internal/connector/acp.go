package connector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"sync/atomic"
)

// ACPConfig Hermes ACP 子进程配置
type ACPConfig struct {
	Command string            `yaml:"command"`           // Hermes CLI 路径
	Args    []string          `yaml:"args"`              // 启动参数，通常为 ["acp"]
	Env     map[string]string `yaml:"env"`               // 追加环境变量
	WorkDir string            `yaml:"work_dir"`          // 指令执行的工作目录，默认连接器启动目录
}

// acpMessage ACP 线路上的 JSON-RPC 消息（请求/响应/通知共用）
type acpMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *acpError       `json:"error,omitempty"`
}

type acpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *acpError) Error() string { return e.Message }

// runACP 启动 hermes acp 子进程、建立会话、下发指令并收集输出。
// mcpServers 以 ACP 格式注入工具桥等 MCP Server（可为 nil）；
// onUpdate 收到会话事件时回调（tool_call 等），用于日志；agent 文本输出作为返回值。
func runACP(ctx context.Context, cfg ACPConfig, instruction string, mcpServers []any, onUpdate func(string)) (string, error) {
	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = "."
	}
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("解析工作目录失败: %w", err)
	}

	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	// Hermes 是 Python 程序：Windows 管道默认走系统区域编码（GBK），
	// 强制 UTF-8 模式，否则中文在 stdio JSON-RPC 里乱码（cfg.Env 可覆盖）
	env := make([]string, 0, len(cfg.Env)+2)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "PYTHONUTF8=") && !strings.HasPrefix(kv, "PYTHONIOENCODING=") {
			env = append(env, kv)
		}
	}
	env = append(env, "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("创建 stdin 管道失败: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("创建 stdout 管道失败: %w", err)
	}
	cmd.Stderr = os.Stderr // Hermes 的日志走 stderr，直接透传便于排查
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动 ACP 子进程失败: %w", err)
	}
	defer func() {
		// 指令结束：关 stdin 让 hermes 退出，5s 不退则强杀（ctx 超时由 CommandContext 兜底）
		_ = stdin.Close()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}()

	var (
		nextID   atomic.Int64
		writeMu  sync.Mutex
		pending  = map[int64]chan acpMessage{}
		pendingM sync.Mutex
	)

	debug := os.Getenv("HERMESHUB_ACP_DEBUG") != ""

	writeJSON := func(v any) error {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if debug {
			log.Printf("[acp] >> %s", data)
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		_, err = stdin.Write(append(data, '\n'))
		return err
	}

	// request 发送 JSON-RPC 请求并等待响应（从读取协程分发）
	request := func(method string, params any) (json.RawMessage, error) {
		id := nextID.Add(1)
		ch := make(chan acpMessage, 1)
		pendingM.Lock()
		pending[id] = ch
		pendingM.Unlock()
		defer func() {
			pendingM.Lock()
			delete(pending, id)
			pendingM.Unlock()
		}()

		if err := writeJSON(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  method,
			"params":  params,
		}); err != nil {
			return nil, err
		}
		select {
		case msg := <-ch:
			if msg.Error != nil {
				return nil, msg.Error
			}
			return msg.Result, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	var outputMu sync.Mutex
	var output bytes.Buffer
	appendOutput := func(text string) {
		outputMu.Lock()
		output.WriteString(text)
		outputMu.Unlock()
	}

	// handleAgentRequest 处理 agent 主动发来的请求（权限、文件读写）
	handleAgentRequest := func(msg acpMessage) {
		var respParams any
		var respErr *acpError
		switch msg.Method {
		case "session/request_permission":
			respParams = handlePermissionRequest(msg.Params)
		case "fs/read_text_file":
			respParams, respErr = handleFSRead(msg.Params)
		case "fs/write_text_file":
			respParams, respErr = handleFSWrite(msg.Params)
		case "fs/create_directory":
			respParams, respErr = handleFSMkdir(msg.Params)
		default:
			respErr = &acpError{Code: -32601, Message: "method not found: " + msg.Method}
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": msg.ID}
		if respErr != nil {
			resp["error"] = map[string]any{"code": respErr.Code, "message": respErr.Message}
		} else {
			resp["result"] = respParams
		}
		if err := writeJSON(resp); err != nil {
			log.Printf("[acp] 回复 agent 请求失败: %v", err)
		}
	}

	// 读取协程：分发响应 / 通知 / agent 请求
	readerErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			if debug {
				log.Printf("[acp] << %s", line)
			}
			var msg acpMessage
			if err := json.Unmarshal(line, &msg); err != nil {
				continue
			}
			switch {
			case msg.ID != 0 && msg.Method != "": // agent → client 请求
				go handleAgentRequest(msg)
			case msg.ID != 0: // 对我们请求的响应
				pendingM.Lock()
				ch, ok := pending[int64(msg.ID)]
				pendingM.Unlock()
				if ok {
					ch <- msg
				}
			default: // 通知 session/update 等
				handleNotification(msg.Params, appendOutput, onUpdate)
			}
		}
		if err := scanner.Err(); err != nil && err != io.EOF {
			readerErr <- err
			return
		}
		readerErr <- io.EOF
	}()

	// initialize 握手
	initResult, err := request("initialize", map[string]any{
		"protocolVersion": 1,
		"clientInfo":      map[string]any{"name": "hermeshub-connector", "version": "1.0.0"},
		"capabilities": map[string]any{
			"fs": map[string]any{
				"readTextFile": true, "writeTextFile": true, "createDirectory": true,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("ACP 握手失败: %w", err)
	}
	var initParsed struct {
		AgentInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"agentInfo"`
	}
	_ = json.Unmarshal(initResult, &initParsed)
	log.Printf("[acp] 握手完成: %s %s", initParsed.AgentInfo.Name, initParsed.AgentInfo.Version)
	_ = writeJSON(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})

	// 新建会话（注入平台工具桥等 MCP Server）
	newResult, err := request("session/new", map[string]any{
		"cwd":        absWorkDir,
		"mcpServers": mcpServers,
	})
	if err != nil {
		return "", fmt.Errorf("创建 ACP 会话失败: %w", err)
	}
	var sess struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(newResult, &sess); err != nil || sess.SessionID == "" {
		return "", fmt.Errorf("ACP 会话响应异常: %s", string(newResult))
	}
	log.Printf("[acp] 会话已建立: %s", sess.SessionID)

	// 下发指令
	promptResult, err := request("session/prompt", map[string]any{
		"sessionId": sess.SessionID,
		"prompt":    []map[string]any{{"type": "text", "text": instruction}},
	})
	if err != nil {
		return output.String(), fmt.Errorf("指令执行失败: %w", err)
	}
	var stop struct {
		StopReason string `json:"stopReason"`
	}
	_ = json.Unmarshal(promptResult, &stop)

	// Hermes 可能在 session/prompt 响应之后才发最后的 agent_message_chunk，
	// 短暂排空输出（输出静止或超时即返回）
	drainDeadline := time.After(3 * time.Second)
	tick := time.NewTicker(200 * time.Millisecond)
	stable := 0
	lastLen := output.Len()
	for {
		select {
		case <-drainDeadline:
			goto drained
		case <-tick.C:
			if output.Len() == lastLen {
				stable++
				if stable >= 2 { // 连续约 400ms 无新输出
					goto drained
				}
			} else {
				stable = 0
				lastLen = output.Len()
			}
		}
	}
drained:
	tick.Stop()
	log.Printf("[acp] 会话结束: stopReason=%s, 输出 %d 字节", stop.StopReason, output.Len())
	return output.String(), nil
}

// handleNotification 处理 session/update 通知：收集文本输出、记录工具调用
func handleNotification(params json.RawMessage, appendOutput func(string), onUpdate func(string)) {
	var n struct {
		SessionID string `json:"sessionId"`
		Update    struct {
			SessionUpdate string          `json:"sessionUpdate"`
			Type          string          `json:"update"` // 兼容早期实现
			Content       json.RawMessage `json:"content"`
			Title         string          `json:"title"`
			Status        string          `json:"status"`
		} `json:"update"`
	}
	if err := json.Unmarshal(params, &n); err != nil {
		return
	}
	updateType := n.Update.SessionUpdate
	if updateType == "" {
		updateType = n.Update.Type
	}
	if updateType == "" {
		return
	}
	switch updateType {
	case "agent_message_chunk":
		var content struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(n.Update.Content, &content) == nil {
			appendOutput(content.Text)
		}
	case "tool_call":
		var call struct {
			Title string `json:"title"`
			Kind  string `json:"kind"`
		}
		if json.Unmarshal(n.Update.Content, &call) == nil {
			title := call.Title
			if title == "" {
				title = call.Kind
			}
			if onUpdate != nil && title != "" {
				onUpdate("tool_call: " + title)
			}
		}
	}
}

// handlePermissionRequest 自动应答权限请求：优先选 allow/approve/yes 选项
func handlePermissionRequest(params json.RawMessage) any {
	var req struct {
		Options []struct {
			OptionID string `json:"optionId"`
			Name     string `json:"name"`
		} `json:"options"`
	}
	_ = json.Unmarshal(params, &req)
	pick := ""
	for _, opt := range req.Options {
		l := strings.ToLower(opt.OptionID + " " + opt.Name)
		if strings.Contains(l, "allow") || strings.Contains(l, "approve") || strings.Contains(l, "yes") {
			pick = opt.OptionID
			break
		}
	}
	if pick == "" && len(req.Options) > 0 {
		pick = req.Options[0].OptionID
	}
	log.Printf("[acp] 自动应答权限请求: %s (%d 个选项)", pick, len(req.Options))
	return map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": pick}}
}

func handleFSRead(params json.RawMessage) (any, *acpError) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Path == "" {
		return nil, &acpError{Code: -32602, Message: "invalid fs/read_text_file params"}
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return nil, &acpError{Code: -32000, Message: err.Error()}
	}
	return map[string]any{"content": string(data)}, nil
}

func handleFSWrite(params json.RawMessage) (any, *acpError) {
	var p struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Path == "" {
		return nil, &acpError{Code: -32602, Message: "invalid fs/write_text_file params"}
	}
	if err := os.WriteFile(p.Path, []byte(p.Content), 0o644); err != nil {
		return nil, &acpError{Code: -32000, Message: err.Error()}
	}
	return map[string]any{}, nil
}

func handleFSMkdir(params json.RawMessage) (any, *acpError) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Path == "" {
		return nil, &acpError{Code: -32602, Message: "invalid fs/create_directory params"}
	}
	if err := os.MkdirAll(p.Path, 0o755); err != nil {
		return nil, &acpError{Code: -32000, Message: err.Error()}
	}
	return map[string]any{}, nil
}
