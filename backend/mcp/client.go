package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// Client MCP stdio 客户端：管理子进程 + JSON-RPC 通信
// 传输方式：newline-delimited JSON（MCP stdio transport）
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader

	mu     sync.Mutex
	nextID int64
	pending map[int64]chan rpcResponse

	serverInfo ServerInfo
	tools      []Tool
}

// ClientConfig 启动参数
type ClientConfig struct {
	Command string   // 如 "npx" 或本地二进制
	Args    []string
	Env     []string // "KEY=VALUE" 形式
	Timeout time.Duration
}

// Start 启动子进程并完成 MCP 握手（initialize）
func (c *Client) Start(ctx context.Context, cfg ClientConfig) error {
	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	cmd.Env = append(append([]string{}, cfg.Env...), "MCP_NON_INTERACTIVE=1")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = io.Discard // stderr 留给调试，暂时丢弃

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 MCP 进程失败: %w", err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.reader = bufio.NewReader(stdout)
	c.pending = make(map[int64]chan rpcResponse)

	go c.readLoop()

	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	hctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	// initialize 握手
	var res initializeResult
	if err := c.call(hctx, "initialize", initializeParams{
		ProtocolVersion: protocolVersion,
		Capabilities:    map[string]any{"tools": map[string]any{}},
		ClientInfo:      ServerInfo{Name: "alice", Version: "0.1.0"},
	}, &res); err != nil {
		c.Close()
		return fmt.Errorf("MCP initialize 失败: %w", err)
	}
	c.serverInfo = res.ServerInfo

	// initialized 通知
	_ = c.notify(ctx, "notifications/initialized", map[string]any{})

	// 拉取工具列表
	var tl toolsListResult
	if err := c.call(ctx, "tools/list", map[string]any{}, &tl); err != nil {
		c.Close()
		return fmt.Errorf("MCP tools/list 失败: %w", err)
	}
	c.tools = tl.Tools

	return nil
}

// readLoop 读取子进程输出，分发给 pending 请求
func (c *Client) readLoop() {
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			c.failPending(err)
			return
		}
		if line == "\n" || line == "" {
			continue
		}
		var msg rpcMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.ID != nil {
			c.mu.Lock()
			ch, ok := c.pending[*msg.ID]
			delete(c.pending, *msg.ID)
			c.mu.Unlock()
			if ok {
				ch <- rpcResponse{JSONRPC: msg.JSONRPC, ID: *msg.ID, Result: msg.Result, Error: msg.Error}
				close(ch)
			}
		}
		// 服务器主动通知（logs 等）暂忽略
	}
}

// call 发送请求并等待响应
func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan rpcResponse, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	req := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("写入失败: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return fmt.Errorf("MCP %s 错误: %s", method, resp.Error.Message)
		}
		if out != nil {
			return json.Unmarshal(resp.Result, out)
		}
		return nil
	}
}

func (c *Client) notify(ctx context.Context, method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	msg := rpcNotification{JSONRPC: "2.0", Method: method, Params: params}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(append(data, '\n'))
	return err
}

func (c *Client) failPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		ch <- rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -1, Message: err.Error()}}
		close(ch)
		delete(c.pending, id)
	}
}

// Tools 返回服务器提供的工具
func (c *Client) Tools() []Tool { return c.tools }

// ServerInfo 返回服务器信息
func (c *Client) ServerInfo() ServerInfo { return c.serverInfo }

// Call 调用工具
func (c *Client) Call(ctx context.Context, name string, args map[string]any) (*CallResult, error) {
	var res CallResult
	if err := c.call(ctx, "tools/call", toolsCallParams{Name: name, Arguments: args}, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// Close 关闭子进程
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
}
