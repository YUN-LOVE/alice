package mcp

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"alice/config"
)

// ManagedServer 一个已安装的 MCP Server 实例
type ManagedServer struct {
	ID        string
	Name      string
	Transport string // "stdio" / "http"
	Command   string
	Args      []string
	Env       []string
	URL       string
	Headers   map[string]string

	mu          sync.Mutex
	conn        MCPConn
	tools       []Tool
	toolEnabled map[string]bool // 工具级开关（缺省启用）
	running     bool
}

// ToolStatus 单个工具的启用状态
type ToolStatus struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// Status MCP Server 状态（HTTP/WS 返回用）
type Status struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Enabled   bool         `json:"enabled"`
	Running   bool         `json:"running"`
	ToolCount int          `json:"tool_count"`
	Tools     []ToolStatus `json:"tools"`
}

// Manager MCP Server 生命周期管理
type Manager struct {
	mu        sync.Mutex
	servers   map[string]*ManagedServer
	autoStart bool
	rdb       *redis.Client
	timeout   time.Duration // 启动超时
}

// NewManager 创建 Manager
func NewManager(autoStart bool, redisAddr, redisPassword string, redisDB int, timeout time.Duration) *Manager {
	m := &Manager{
		servers:   make(map[string]*ManagedServer),
		autoStart: autoStart,
		timeout:   timeout,
	}
	if timeout <= 0 {
		m.timeout = 30 * time.Second
	}
	if redisAddr != "" {
		m.rdb = redis.NewClient(&redis.Options{
			Addr:     redisAddr,
			Password: redisPassword,
			DB:       redisDB,
		})
	}
	return m
}

// Add 注册一个已配置的 Server
func (m *Manager) Add(id, name string, cfg config.MCPServerConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers[id] = &ManagedServer{
		ID: id, Name: name,
		Transport: cfg.Transport,
		Command:   cfg.Command,
		Args:      cfg.Args,
		Env:       cfg.Env,
		URL:       cfg.URL,
		Headers:   cfg.Headers,
	}
}

// Reload 热重载：按新配置整体重建（停止旧 Server，按新配置重新启动）
func (m *Manager) Reload(ctx context.Context, servers []config.MCPServerConfig) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.servers))
	for id := range m.servers {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	for _, id := range ids {
		m.Stop(id)
	}

	m.mu.Lock()
	m.servers = make(map[string]*ManagedServer)
	m.mu.Unlock()

	for _, s := range servers {
		m.Add(s.ID, s.Name, s)
	}
	m.StartAll(ctx)
	log.Printf("[mcp] 配置热重载完成，共 %d 个 Server", len(servers))
}

// StartAll 启动所有已启用的 Server
func (m *Manager) StartAll(ctx context.Context) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.servers))
	for id, s := range m.servers {
		if m.autoStart {
			ids = append(ids, id)
		}
		_ = s
	}
	m.mu.Unlock()

	for _, id := range ids {
		if err := m.Start(ctx, id); err != nil {
			log.Printf("[mcp] Server %s 启动失败: %v", id, err)
		} else {
			log.Printf("[mcp] Server %s 已启动", id)
		}
	}
}

// Start 启动指定 Server 并拉取工具
func (m *Manager) Start(ctx context.Context, id string) error {
	m.mu.Lock()
	s, ok := m.servers[id]
	m.mu.Unlock()
	if !ok {
		return errServerNotFound(id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}

	var conn MCPConn
	cfg := ClientConfig{Timeout: m.timeout}
	if s.Transport == "http" {
		conn = &HTTPClient{}
		cfg.URL = s.URL
		cfg.Headers = s.Headers
	} else {
		conn = &Client{}
		cfg.Command = s.Command
		cfg.Args = s.Args
		cfg.Env = s.Env
	}
	if err := conn.Start(ctx, cfg); err != nil {
		return err
	}
	s.conn = conn
	s.tools = conn.Tools()
	s.toolEnabled = make(map[string]bool, len(s.tools))
	for _, t := range s.tools {
		s.toolEnabled[t.Name] = true
	}
	s.running = true
	// 从 Redis 恢复工具级开关
	m.loadToolStatesLocked(s)
	return nil
}

// toolStateKey Redis key：server 下各工具的启用状态
func toolStateKey(serverID string) string { return "alice:mcp:tools:" + serverID }

// loadToolStatesLocked 从 Redis 读取该 server 的工具开关（无记录则保持启用）
func (m *Manager) loadToolStatesLocked(s *ManagedServer) {
	if m.rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	states, err := m.rdb.HGetAll(ctx, toolStateKey(s.ID)).Result()
	if err != nil {
		return
	}
	for name, val := range states {
		if _, exists := s.toolEnabled[name]; exists {
			s.toolEnabled[name] = val == "1"
		}
	}
}

// SetToolEnabled 设置工具级启用状态（持久化）
func (m *Manager) SetToolEnabled(serverID, toolName string, enabled bool) error {
	m.mu.Lock()
	s, ok := m.servers[serverID]
	m.mu.Unlock()
	if !ok {
		return errServerNotFound(serverID)
	}

	s.mu.Lock()
	if s.toolEnabled == nil {
		s.toolEnabled = make(map[string]bool)
	}
	s.toolEnabled[toolName] = enabled
	s.mu.Unlock()

	if m.rdb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		val := "0"
		if enabled {
			val = "1"
		}
		if err := m.rdb.HSet(ctx, toolStateKey(serverID), toolName, val).Err(); err != nil {
			return err
		}
	}
	return nil
}

// Stop 停止 Server
func (m *Manager) Stop(id string) {
	m.mu.Lock()
	s, ok := m.servers[id]
	m.mu.Unlock()
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running && s.conn != nil {
		s.conn.Close()
	}
	s.running = false
	s.conn = nil
	s.tools = nil
}

// Tools 返回所有运行中 Server 的已启用工具（带 serverID 前缀）
func (m *Manager) Tools() []Tool {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Tool
	for _, s := range m.servers {
		s.mu.Lock()
		if s.running {
			for _, t := range s.tools {
				if s.toolEnabled != nil && !s.toolEnabled[t.Name] {
					continue // 工具级关闭
				}
				t.Name = s.ID + "__" + t.Name
				out = append(out, t)
			}
		}
		s.mu.Unlock()
	}
	return out
}

// Call 调用工具（工具名格式：serverID__toolName）
func (m *Manager) Call(ctx context.Context, fullName string, args map[string]any) (string, error) {
	sep := strings.Index(fullName, "__")
	if sep <= 0 {
		return "", errServerNotFound(fullName)
	}
	srvID, toolName := fullName[:sep], fullName[sep+2:]

	m.mu.Lock()
	s, ok := m.servers[srvID]
	m.mu.Unlock()
	if !ok {
		return "", errServerNotFound(srvID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.conn == nil {
		return "", errServerNotRunning(srvID)
	}
	res, err := s.conn.Call(ctx, toolName, args)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	if res.IsError {
		return sb.String(), errToolError(sb.String())
	}
	return sb.String(), nil
}

// Status 返回所有 Server 状态（含工具级开关）
func (m *Manager) Status() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Status, 0, len(m.servers))
	for _, s := range m.servers {
		s.mu.Lock()
		tools := make([]ToolStatus, 0, len(s.tools))
		for _, t := range s.tools {
			enabled := true
			if s.toolEnabled != nil {
				enabled = s.toolEnabled[t.Name]
			}
			tools = append(tools, ToolStatus{Name: t.Name, Enabled: enabled})
		}
		out = append(out, Status{
			ID: s.ID, Name: s.Name, Enabled: true, Running: s.running,
			ToolCount: len(tools), Tools: tools,
		})
		s.mu.Unlock()
	}
	return out
}

func errServerNotFound(id string) error { return &MCPError{"服务器不存在: " + id} }
func errServerNotRunning(id string) error { return &MCPError{"服务器未运行: " + id} }
func errToolError(msg string) error { return &MCPError{"工具调用错误: " + msg} }

// MCPError MCP 错误类型
type MCPError struct{ Message string }

func (e *MCPError) Error() string { return e.Message }
