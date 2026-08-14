package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"alice/config"
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

	uploadMu sync.Mutex
	upload   *UploadState // 进行中的分块上传（每连接同时最多一个）
}

// UploadState 一次分块上传的进行中状态
type UploadState struct {
	file        *os.File
	path        string
	fileName    string
	totalChunks int
	received    int
	bytes       int64
}

func (u *UploadState) abort() {
	if u.file != nil {
		_ = u.file.Close()
	}
	_ = os.Remove(u.path)
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
		msgType, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		// 二进制帧：分块上传内容（协议：upload_start → N 个 Binary Frame → upload_end）
		if msgType == websocket.BinaryMessage {
			if err := c.handleUploadChunk(data); err != nil {
				c.sendError(err.Error())
			}
			continue
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
		broadcastMCPCapabilities(c.hub, c.kernel)
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
		if err == nil {
			broadcastMCPCapabilities(c.hub, c.kernel)
		}
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
		if err == nil {
			broadcastMCPCapabilities(c.hub, c.kernel)
		}
		return c.sendJSON(WsMessage{Type: "mcp_install_ack", Payload: mustJSON(map[string]any{"id": p.ID, "ok": err == nil, "message": errMsg(err)})})

	case "mcp_uninstall":
		var p struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			return err
		}
		err := c.kernel.MCPUninstall(p.ID)
		if err == nil {
			broadcastMCPCapabilities(c.hub, c.kernel)
		}
		return c.sendJSON(WsMessage{Type: "mcp_uninstall_ack", Payload: mustJSON(map[string]any{"id": p.ID, "ok": err == nil, "message": errMsg(err)})})

	case "mcp_configure":
		var p struct {
			ID     string                 `json:"id"`
			Config config.MCPServerConfig `json:"config"`
		}
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			return err
		}
		err := c.kernel.MCPConfigure(p.ID, p.Config)
		if err == nil {
			broadcastMCPCapabilities(c.hub, c.kernel)
		}
		return c.sendJSON(WsMessage{Type: "mcp_configure_ack", Payload: mustJSON(map[string]any{
			"id": p.ID, "ok": err == nil, "message": errMsg(err),
		})})

	case "settings_update":
		var p struct {
			Section string         `json:"section"`
			Values  map[string]any `json:"values"`
		}
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			return err
		}
		err := c.kernel.ApplySettings(p.Section, p.Values)
		return c.sendJSON(WsMessage{Type: "settings_update_ack", Payload: mustJSON(map[string]any{
			"section": p.Section, "ok": err == nil, "message": errMsg(err),
		})})

	case "upload_start":
		var p struct {
			UploadID    string `json:"upload_id"`
			FileName    string `json:"file_name"`
			TotalChunks int    `json:"total_chunks"`
		}
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			return err
		}
		return c.startUpload(p.UploadID, p.FileName, p.TotalChunks)

	case "upload_end":
		return c.finishUpload()

	default:
		return nil
	}
}

// ==================== 文件分块上传 ====================

// startUpload 开始一次分块上传：创建目标文件（文件名清洗防路径穿越）
func (c *Client) startUpload(uploadID, fileName string, totalChunks int) error {
	c.uploadMu.Lock()
	defer c.uploadMu.Unlock()
	if c.upload != nil {
		return fmt.Errorf("已有进行中的上传，请先 upload_end")
	}
	if totalChunks <= 0 || fileName == "" {
		return fmt.Errorf("upload_start 参数无效")
	}
	dir := filepath.Join(kernel.UploadsDir(), "files")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	base := filepath.Base(fileName) // 防路径穿越
	if base == "." || base == "/" || base == "" {
		base = "upload"
	}
	path := filepath.Join(dir, fmt.Sprintf("%s_%s", time.Now().Format("20060102_150405"), base))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	c.upload = &UploadState{file: f, path: path, fileName: base, totalChunks: totalChunks}
	_ = uploadID // 预留：多端并发上传时用 upload_id 区分
	return nil
}

// handleUploadChunk 追加一个二进制分块
func (c *Client) handleUploadChunk(data []byte) error {
	c.uploadMu.Lock()
	defer c.uploadMu.Unlock()
	if c.upload == nil {
		return fmt.Errorf("没有进行中的上传（先发 upload_start）")
	}
	u := c.upload
	if maxBytes := c.kernel.MaxUploadBytes(); maxBytes > 0 && u.bytes+int64(len(data)) > maxBytes {
		u.abort()
		c.upload = nil
		return fmt.Errorf("上传超过大小上限")
	}
	if _, err := u.file.Write(data); err != nil {
		u.abort()
		c.upload = nil
		return err
	}
	u.bytes += int64(len(data))
	u.received++
	return nil
}

// finishUpload 结束上传：校验分块完整性，返回文件 URL 路径
func (c *Client) finishUpload() error {
	c.uploadMu.Lock()
	defer c.uploadMu.Unlock()
	u := c.upload
	if u == nil {
		return fmt.Errorf("没有进行中的上传")
	}
	c.upload = nil
	if err := u.file.Close(); err != nil {
		_ = os.Remove(u.path)
		return err
	}
	if u.received != u.totalChunks {
		_ = os.Remove(u.path)
		return fmt.Errorf("分块不完整: %d/%d，已丢弃", u.received, u.totalChunks)
	}
	return c.sendJSON(WsMessage{Type: "upload_complete_ack", Payload: mustJSON(map[string]any{
		"ok":        true,
		"path":      "/uploads/files/" + filepath.Base(u.path),
		"file_name": u.fileName,
		"size":      u.bytes,
	})})
}

// broadcastMCPCapabilities 广播 MCP 能力状态到所有连接（前端实时刷新）
func broadcastMCPCapabilities(hub *Hub, k *kernel.Kernel) {
	if hub == nil {
		return
	}
	hub.Broadcast(WsMessage{Type: "mcp_capabilities", Payload: mustJSON(map[string]any{
		"servers": k.MCPStatus(),
	})})
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
