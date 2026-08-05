package mcp

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"alice/config"
)

// buildLocalTools 编译内置测试 MCP Server 到临时目录
func buildLocalTools(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "local-tools")
	_, thisFile, _, _ := runtime.Caller(0)
	src := filepath.Join(filepath.Dir(thisFile), "..", "..", "mcp-server")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = src
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("编译内置 MCP Server 失败: %v\n%s", err, out)
	}
	return bin
}

// TestClientLifecycle 启动 → 握手 → 工具列表 → 调用，完整协议链路
func TestClientLifecycle(t *testing.T) {
	bin := buildLocalTools(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c := &Client{}
	if err := c.Start(ctx, ClientConfig{Command: bin}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer c.Close()

	if c.ServerInfo().Name != "alice-local-tools" {
		t.Fatalf("ServerInfo 错误: %v", c.ServerInfo())
	}

	tools := c.Tools()
	if len(tools) != 3 {
		t.Fatalf("应暴露 3 个工具，实际 %d", len(tools))
	}

	// 调用 calculator
	res, err := c.Call(ctx, "calculator", map[string]any{"expression": "(1+2)*3"})
	if err != nil {
		t.Fatalf("calculator 调用失败: %v", err)
	}
	if !containsText(res, "9") {
		t.Fatalf("calculator 结果错误: %+v", res.Content)
	}

	// 调用 get_time
	res, err = c.Call(ctx, "get_time", map[string]any{})
	if err != nil {
		t.Fatalf("get_time 调用失败: %v", err)
	}
	if len(res.Content) == 0 || res.Content[0].Text == "" {
		t.Fatalf("get_time 结果为空")
	}
}

// TestManagerCall 通过 Manager 分发（serverID__toolName）
func TestManagerCall(t *testing.T) {
	bin := buildLocalTools(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	m := NewManager(true, "", "", 0)
	m.Add("local", "本地工具", config.MCPServerConfig{
		Command: bin,
		Enabled: true,
	})
	m.StartAll(ctx)

	tools := m.Tools()
	if len(tools) != 3 {
		t.Fatalf("Manager 工具数错误: %d", len(tools))
	}
	// 工具名应带前缀
	found := false
	for _, t := range tools {
		if t.Name == "local__calculator" {
			found = true
		}
	}
	if !found {
		t.Fatal("工具名未带 serverID 前缀")
	}

	result, err := m.Call(ctx, "local__calculator", map[string]any{"expression": "2+3"})
	if err != nil {
		t.Fatalf("Manager.Call 失败: %v", err)
	}
	if !containsText(&CallResult{Content: []ContentBlock{{Type: "text", Text: result}}}, "5") {
		t.Fatalf("Manager.Call 结果错误: %s", result)
	}

	// 停止后工具应清空
	m.Stop("local")
	if len(m.Tools()) != 0 {
		t.Fatal("停止后工具应清空")
	}
}

func containsText(res *CallResult, sub string) bool {
	for _, c := range res.Content {
		if c.Type == "text" && len(c.Text) > 0 && (len(sub) == 0 || contains(c.Text, sub)) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
