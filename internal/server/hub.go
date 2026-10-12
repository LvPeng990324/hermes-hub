package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"hermeshub/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

// WS 消息类型（平台 ↔ 连接器）
const (
	// 平台 → 连接器
	WSTypeDispatch = "dispatch" // 下发指令
	WSTypePing     = "ping"
	// 连接器 → 平台
	WSTypeAck    = "ack"    // 确认收到指令
	WSTypeResult = "result" // 指令执行结果
	WSTypePong   = "pong"
)

// wsInMessage 连接器上行消息
type wsInMessage struct {
	Type       string `json:"type"`
	DispatchID uint   `json:"dispatch_id"`
	Status     string `json:"status"` // done / failed
	Output     string `json:"output"`
	Error      string `json:"error"`
}

// wsOutMessage 平台下行消息
type wsOutMessage struct {
	Type        string `json:"type"`
	DispatchID  uint   `json:"dispatch_id,omitempty"`
	Instruction string `json:"instruction,omitempty"`
	ToolToken   string `json:"tool_token,omitempty"` // 当次派发的「实例×项目」工具令牌（供连接器注入工具桥）
}

type wsConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *wsConn) writeJSON(v any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteJSON(v)
}

// Hub 管理所有连接器的 WebSocket 长连接
type Hub struct {
	db    *gorm.DB
	mu    sync.RWMutex
	conns map[uint]*wsConn // instanceID -> 连接
	orch  *Orchestrator
}

func NewHub(db *gorm.DB) *Hub {
	return &Hub{db: db, conns: make(map[uint]*wsConn)}
}

func (h *Hub) IsOnline(instanceID uint) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.conns[instanceID]
	return ok
}

// OnlineIDs 当前在线实例 ID 集合
func (h *Hub) OnlineIDs() map[uint]bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[uint]bool, len(h.conns))
	for id := range h.conns {
		out[id] = true
	}
	return out
}

// SendDispatch 向指定实例的连接器推送指令；实例离线时返回错误
func (h *Hub) SendDispatch(instanceID uint, dispatchID uint, instruction string, toolToken string) error {
	h.mu.RLock()
	wc, ok := h.conns[instanceID]
	h.mu.RUnlock()
	if !ok {
		return errOffline
	}
	return wc.writeJSON(wsOutMessage{Type: WSTypeDispatch, DispatchID: dispatchID, Instruction: instruction, ToolToken: toolToken})
}

var errOffline = errOfflineError{}

type errOfflineError struct{}

func (errOfflineError) Error() string { return "实例离线" }

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// HandleConnectorWS 连接器接入入口：GET /ws/connector?token=xxx
func (h *Hub) HandleConnectorWS(c *gin.Context) {
	token := c.Query("token")
	var inst models.Instance
	if err := h.db.Where("token = ?", token).First(&inst).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的接入令牌"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[hub] websocket upgrade 失败: %v", err)
		return
	}
	wc := &wsConn{conn: conn}

	h.mu.Lock()
	if old, ok := h.conns[inst.ID]; ok {
		_ = old.conn.Close() // 同实例重复接入，踢掉旧连接
	}
	h.conns[inst.ID] = wc
	h.mu.Unlock()
	log.Printf("[hub] 实例 %s (ID=%d) 已接入", inst.Name, inst.ID)

	defer func() {
		h.mu.Lock()
		if h.conns[inst.ID] == wc {
			delete(h.conns, inst.ID)
		}
		h.mu.Unlock()
		_ = conn.Close()
		log.Printf("[hub] 实例 %s (ID=%d) 已断开", inst.Name, inst.ID)
	}()

	// 服务端心跳：30s 一次应用层 ping；连接器的 app 层 pong 由读循环续超时，协议层 pong 兜底
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	})
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if err := wc.writeJSON(wsOutMessage{Type: WSTypePing}); err != nil {
					return
				}
			}
		}
	}()
	defer close(stop)

	// 读循环：处理 ack / result；收到任何消息都续读超时
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		var msg wsInMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case WSTypeAck:
			h.orch.OnAck(msg.DispatchID)
		case WSTypeResult:
			h.orch.OnResult(msg.DispatchID, msg.Status, msg.Output, msg.Error)
		case WSTypePong:
			// 连接器主动心跳，无需处理
		}
	}
}
