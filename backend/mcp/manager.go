package mcp

import (
	"context"
	"log"
	"strings"
	"sync"
)

// ManagedServer 一个已安装的 MCP Server 实例
type ManagedServer struct {
	ID      string
	Name    string
	Command string
	Args    []string
	Env     []string

	mu      sync.Mutex
	client  *Client
	tools   []Tool
	running bool
}

// Status MCP Server 状态（HTTP/WS 返回用）
type Status struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	Running  bool   `json:"running"`
	ToolCount int   `json:"tool_count"`
}

// Manager MCP Server 生命周期管理
type Manager struct {
	mu      sync.Mutex
	servers map[string]*ManagedServer
	autoStart bool
}

// NewManager 创建 Manager
func NewManager(autoStart bool) *Manager {
	return &Manager{
		servers:   make(map[string]*ManagedServer),
		autoStart: autoStart,
	}
}

// Add 注册一个已配置的 Server
func (m *Manager) Add(id, name, command string, args, env []string, enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers[id] = &ManagedServer{
		ID: id, Name: name, Command: command, Args: args, Env: env,
	}
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

	c := &Client{}
	if err := c.Start(ctx, ClientConfig{Command: s.Command, Args: s.Args, Env: s.Env}); err != nil {
		return err
	}
	s.client = c
	s.tools = c.Tools()
	s.running = true
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
	if s.running && s.client != nil {
		s.client.Close()
	}
	s.running = false
	s.client = nil
	s.tools = nil
}

// Tools 返回所有运行中 Server 的工具（带 serverID 前缀）
func (m *Manager) Tools() []Tool {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Tool
	for _, s := range m.servers {
		s.mu.Lock()
		if s.running {
			for _, t := range s.tools {
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
	if !s.running || s.client == nil {
		return "", errServerNotRunning(srvID)
	}
	res, err := s.client.Call(ctx, toolName, args)
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

// Status 返回所有 Server 状态
func (m *Manager) Status() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Status, 0, len(m.servers))
	for _, s := range m.servers {
		s.mu.Lock()
		out = append(out, Status{
			ID: s.ID, Name: s.Name, Enabled: true, Running: s.running, ToolCount: len(s.tools),
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
