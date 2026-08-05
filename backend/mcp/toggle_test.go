package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"alice/config"
)

// TestToolToggle 工具级开关：关闭单个工具后 Tools() 应过滤
func TestToolToggle(t *testing.T) {
	bin := buildLocalTools(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	m := NewManager(true, "", "", 0)
	m.Add("local", "本地工具", config.MCPServerConfig{Command: bin, Enabled: true})
	m.StartAll(ctx)

	if got := len(m.Tools()); got != 3 {
		t.Fatalf("默认应暴露 3 个工具，实际 %d", got)
	}

	// 关闭 calculator
	if err := m.SetToolEnabled("local", "calculator", false); err != nil {
		t.Fatalf("设置工具开关失败: %v", err)
	}
	tools := m.Tools()
	if len(tools) != 2 {
		t.Fatalf("关闭后应剩 2 个工具，实际 %d: %v", len(tools), tools)
	}
	for _, tl := range tools {
		if tl.Name == "local__calculator" {
			t.Fatal("calculator 应被过滤掉")
		}
	}

	// Status 应反映工具状态
	status := m.Status()
	for _, s := range status {
		if s.ID == "local" {
		for _, ts := range s.Tools {
			if ts.Name == "calculator" && ts.Enabled {
				t.Fatal("Status 中 calculator 应为禁用")
			}
		}
		}
	}

	// 重新启用
	if err := m.SetToolEnabled("local", "calculator", true); err != nil {
		t.Fatalf("重新启用失败: %v", err)
	}
	if got := len(m.Tools()); got != 3 {
		t.Fatalf("重新启用后应 3 个工具，实际 %d", got)
	}
}

// TestRegistryLoad 注册表加载与查询
func TestRegistryLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	content := `{
	  "version": "1.0",
	  "servers": [
	    {"id": "a", "name": "工具A", "description": "测试", "command": "npx", "args": ["-y", "pkg-a"]},
	    {"id": "b", "name": "工具B", "description": "测试", "command": "/path/b"}
	  ]
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := LoadRegistry("file", path)
	if err != nil {
		t.Fatalf("加载注册表失败: %v", err)
	}
	if len(reg.Servers) != 2 {
		t.Fatalf("注册表应有 2 项，实际 %d", len(reg.Servers))
	}
	item, ok := reg.Item("a")
	if !ok || item.Name != "工具A" {
		t.Fatalf("按 ID 查找失败: %v", item)
	}
	if _, ok := reg.Item("nonexist"); ok {
		t.Fatal("不存在的项不应命中")
	}
	cfg := item.ToConfig()
	if cfg.Transport != "" {
		t.Fatalf("默认 transport 应为空（stdio），实际 %q", cfg.Transport)
	}
}
