package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"alice/kernel"
)

// WsMessage 前后端统一消息结构
type WsMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Hub 连接注册表：支持主动广播（情绪推送等）
type Hub struct {
	mu      sync.Mutex
	clients map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*Client]struct{})}
}

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
}

// Broadcast 向所有连接广播消息
func (h *Hub) Broadcast(msg WsMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.send <- data:
		default:
		}
	}
}

// Client 单个 WebSocket 连接
type Client struct {
	conn      *websocket.Conn
	send      chan []byte
	kernel    *kernel.Kernel
	hub       *Hub
	close     chan struct{}
	once      sync.Once
	sessionID string
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 54 * time.Second
)

// HandleWebSocket 升级 HTTP 连接为 WebSocket
func HandleWebSocket(hub *Hub, k *kernel.Kernel) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("[ws] 升级失败: %v", err)
			return
		}
		c := &Client{
			conn:   conn,
			send:   make(chan []byte, 256),
			kernel: k,
			hub:    hub,
			close:  make(chan struct{}),
		}
		hub.register(c)
		go c.writePump()
		go c.readPump()
		log.Printf("[ws] 新连接: %s（当前 %d 个连接）", r.RemoteAddr, hub.count())
	}
}

// readPump 处理前端消息
func (c *Client) readPump() {
	defer func() {
		c.shutdown()
		if c.hub != nil {
			c.hub.unregister(c)
		}
		c.conn.Close()
	}()
	c.conn.SetReadLimit(64 * 1024)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg WsMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.sendError("消息格式错误: " + err.Error())
			continue
		}
		if err := c.handle(msg); err != nil {
			c.sendError(err.Error())
		}
	}
}

func (c *Client) handle(msg WsMessage) error {
	switch msg.Type {
	case "handshake":
		var p struct {
			SessionID string `json:"session_id"`
		}
		if len(msg.Payload) > 0 {
			_ = json.Unmarshal(msg.Payload, &p)
		}
		if p.SessionID != "" {
			c.sessionID = p.SessionID
		}
		return c.sendJSON(WsMessage{Type: "handshake_ack", Payload: mustJSON(map[string]any{
			"name":       "Alice",
			"llm":        c.kernel.LLMName(),
			"version":    "0.1.0",
			"serverTime": time.Now().Unix(),
			"session_id": c.sessionID,
		})})

	case "user_message":
		var p struct {
			Text      string `json:"text"`
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			return err
		}
		if p.Text == "" {
			return nil
		}
		if p.SessionID != "" {
			c.sessionID = p.SessionID
		}
		return c.handleUserMessage(p.Text)

	case "ping":
		return c.sendJSON(WsMessage{Type: "pong", Payload: mustJSON(map[string]any{"time": time.Now().Unix()})})

	case "mcp_installed_list":
		return c.sendJSON(WsMessage{Type: "mcp_installed_list_ack", Payload: mustJSON(map[string]any{"servers": c.kernel.MCPStatus()})})

	case "mcp_toggle":
		var p struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			return err
		}
		if p.Enabled {
			if err := c.kernel.MCPStart(context.Background(), p.ID); err != nil {
				return c.sendJSON(WsMessage{Type: "mcp_toggle_ack", Payload: mustJSON(map[string]any{"id": p.ID, "ok": false, "message": err.Error()})})
			}
		} else {
			c.kernel.MCPStop(p.ID)
		}
		return c.sendJSON(WsMessage{Type: "mcp_toggle_ack", Payload: mustJSON(map[string]any{"id": p.ID, "ok": true, "enabled": p.Enabled})})

	case "mcp_tool_toggle":
		var p struct {
			Server  string `json:"server"`
			Tool    string `json:"tool"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			return err
		}
		err := c.kernel.MCPToolToggle(p.Server, p.Tool, p.Enabled)
		return c.sendJSON(WsMessage{Type: "mcp_tool_toggle_ack", Payload: mustJSON(map[string]any{
			"server": p.Server, "tool": p.Tool, "enabled": p.Enabled, "ok": err == nil,
			"message": errMsg(err),
		})})

	case "mcp_market_list":
		items := []map[string]any{}
		if reg := c.kernel.MCPRegistry(); reg != nil {
			for _, it := range reg.Servers {
				installed := false
				for _, s := range mcpInstalledIDs(c.kernel) {
					if s == it.ID {
						installed = true
						break
					}
				}
				items = append(items, map[string]any{
					"id": it.ID, "name": it.Name, "description": it.Description,
					"installed": installed,
				})
			}
		}
		return c.sendJSON(WsMessage{Type: "mcp_market_list_ack", Payload: mustJSON(map[string]any{"items": items})})

	case "mcp_install":
		var p struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			return err
		}
		err := c.kernel.MCPInstall(p.ID)
		return c.sendJSON(WsMessage{Type: "mcp_install_ack", Payload: mustJSON(map[string]any{"id": p.ID, "ok": err == nil, "message": errMsg(err)})})

	case "mcp_uninstall":
		var p struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			return err
		}
		err := c.kernel.MCPUninstall(p.ID)
		return c.sendJSON(WsMessage{Type: "mcp_uninstall_ack", Payload: mustJSON(map[string]any{"id": p.ID, "ok": err == nil, "message": errMsg(err)})})

	default:
		return nil
	}
}

func (h *Hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// handleUserMessage 走 Kernel 主流程，流式转发回复
func (c *Client) handleUserMessage(text string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := c.kernel.Process(ctx, c.sessionID, text)
	if err != nil {
		return err
	}

	for chunk := range ch {
		msg := WsMessage{Type: "assistant_chunk"}
		msg.Payload = mustJSON(map[string]any{
			"content": chunk.Content,
			"done":    chunk.Done,
		})
		if err := c.sendJSON(msg); err != nil {
			cancel()
			return err
		}
		if chunk.Done {
			// 回复结束，推送情绪状态更新
			c.pushEmotion()
		}
	}
	return nil
}

// pushEmotion 推送当前情绪状态给该客户端
func (c *Client) pushEmotion() {
	state := c.kernel.Emotion().State()
	desc, _, top := c.kernel.Emotion().Summary()
	_ = c.sendJSON(WsMessage{Type: "emotion_update", Payload: mustJSON(map[string]any{
		"state":       state,
		"description": desc,
		"top":         top,
	})})
}

// writePump 统一发送队列 + 心跳
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case data, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.close:
			return
		}
	}
}

func (c *Client) sendJSON(msg WsMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	select {
	case c.send <- data:
		return nil
	default:
		return nil
	}
}

func (c *Client) sendError(text string) {
	_ = c.sendJSON(WsMessage{Type: "error", Payload: mustJSON(map[string]any{"message": text})})
}

func (c *Client) shutdown() {
	c.once.Do(func() { close(c.close) })
}

func mustJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

// mcpInstalledIDs 已安装的 MCP Server ID 列表
func mcpInstalledIDs(k *kernel.Kernel) []string {
	ids := make([]string, 0)
	for _, s := range k.MCPStatus() {
		ids = append(ids, s.ID)
	}
	return ids
}

func errMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
