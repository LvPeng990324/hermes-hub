package connector

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Run 启动连接器：hermeshub-connector --config config.yaml
func Run(args []string) error {
	fs := flag.NewFlagSet("connector", flag.ContinueOnError)
	confPath := fs.String("config", "connector.yaml", "连接器配置文件路径")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := LoadConfig(*confPath)
	if err != nil {
		return err
	}

	c := &Connector{cfg: cfg}
	return c.Loop()
}

// Connector 平台 ↔ Hermes 之间的连接器
type Connector struct {
	cfg *Config
}

type serverMessage struct {
	Type        string `json:"type"`
	DispatchID  uint   `json:"dispatch_id"`
	Instruction string `json:"instruction"`
	ToolToken   string `json:"tool_token"` // 本次派发的「实例×项目」工具令牌
	TestID      string `json:"test_id"`
}

// Loop 长连接主循环：断线自动重连
func (c *Connector) Loop() error {
	for {
		err := c.serve()
		log.Printf("[connector] 与平台断开: %v，%ds 后重连", err, c.cfg.ReconnectSeconds)
		time.Sleep(time.Duration(c.cfg.ReconnectSeconds) * time.Second)
	}
}

func (c *Connector) wsURL() (string, error) {
	u, err := url.Parse(c.cfg.PlatformURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("platform_url 协议不支持: %s", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/ws/connector"
	u.RawQuery = "token=" + url.QueryEscape(c.cfg.Token)
	return u.String(), nil
}

func (c *Connector) serve() error {
	wsURL, err := c.wsURL()
	if err != nil {
		return err
	}
	log.Printf("[connector] 正在连接平台 %s ...", c.cfg.PlatformURL)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("连接平台失败: %w", err)
	}
	defer conn.Close()
	log.Printf("[connector] 已接入平台，等待指令")

	// 心跳：平台 30s 一次应用层 ping；收到任何消息都续读超时（协议层 pong 仅为兜底）
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	})
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))

	var writeMu = make(chan struct{}, 1) // 写锁（容量1信号量）
	writeJSON := func(v any) error {
		writeMu <- struct{}{}
		defer func() { <-writeMu }()
		return conn.WriteJSON(v)
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		var msg serverMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "ping":
			_ = writeJSON(map[string]any{"type": "pong"})
		case "test":
			log.Printf("[connector] 收到连接测试 (test_id=%s)", msg.TestID)
			_ = writeJSON(map[string]any{"type": "test_result", "test_id": msg.TestID})
		case "dispatch":
			if msg.DispatchID == 0 {
				continue
			}
			_ = writeJSON(map[string]any{"type": "ack", "dispatch_id": msg.DispatchID})
			go c.execute(msg.DispatchID, msg.Instruction, msg.ToolToken, writeJSON)
		}
	}
}

// execute 调用 Hermes 执行指令并回传结果
func (c *Connector) execute(dispatchID uint, instruction, toolToken string, writeJSON func(any) error) {
	log.Printf("[connector] 收到指令 (dispatch=%d)，启动 ACP 会话执行", dispatchID)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	onUpdate := func(event string) {
		log.Printf("[connector] dispatch=%d %s", dispatchID, event)
	}
	output, err := runACP(ctx, c.cfg.ACP, instruction, c.mcpServers(toolToken), onUpdate)

	result := map[string]any{"type": "result", "dispatch_id": dispatchID}
	if err != nil {
		log.Printf("[connector] 指令执行失败 (dispatch=%d): %v", dispatchID, err)
		result["status"] = "failed"
		result["error"] = err.Error()
	} else {
		log.Printf("[connector] 指令执行完成 (dispatch=%d)，输出 %d 字节", dispatchID, len(output))
		result["status"] = "done"
		result["output"] = output
	}
	if werr := writeJSON(result); werr != nil {
		log.Printf("[connector] 回传结果失败 (dispatch=%d): %v", dispatchID, werr)
	}
}

// mcpServers 组装注入 ACP 会话的 MCP Server 清单：
// hermeshub 工具桥由 hermes 以子进程方式拉起（sidecar，复用本二进制），
// 平台地址与当次派发的工具令牌经环境变量传入。
// 注意：hermes 的 ACP 实现对 mcpServers 条目采用扁平结构（字段在顶层，
// 与 ACP 规范的 server 嵌套不同），此处按 hermes 实际校验模型组装。
func (c *Connector) mcpServers(toolToken string) []any {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		log.Printf("[connector] 无法定位自身路径，工具桥注入跳过: %v", err)
		return nil
	}
	return []any{
		map[string]any{
			"name":    "hermeshub",
			"type":    "stdio",
			"command": exe,
			"args":    []string{"toolbridge"},
			// hermes 的 ACP 实现要求 env 为 EnvVariable 对象数组（name/value 字典）
			"env": []map[string]string{
				{"name": "HERMESHUB_PLATFORM_URL", "value": c.cfg.PlatformURL},
				{"name": "HERMESHUB_TOOL_TOKEN", "value": toolToken},
			},
		},
	}
}
