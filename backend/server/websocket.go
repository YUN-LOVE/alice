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

// Client 单个 WebSocket 连接
type Client struct {
	conn      *websocket.Conn
	send      chan []byte
	kernel    *kernel.Kernel
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
func HandleWebSocket(k *kernel.Kernel) http.HandlerFunc {
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
			close:  make(chan struct{}),
		}
		go c.writePump()
		go c.readPump()
		log.Printf("[ws] 新连接: %s", r.RemoteAddr)
	}
}

// readPump 处理前端消息
func (c *Client) readPump() {
	defer func() {
		c.shutdown()
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

	default:
		return nil
	}
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
	}
	return nil
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
