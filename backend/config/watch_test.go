package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestWatchDetectsChange 修改配置后 Watch 应触发 onReload
func TestWatchDetectsChange(t *testing.T) {
	dir := t.TempDir()
	writeFullFixture(t, dir)
	file := filepath.Join(dir, "kernel.yaml")

	reloaded := make(chan struct{}, 10)
	Watch(dir, func(_ *Config) {
		select {
		case reloaded <- struct{}{}:
		default:
		}
	})

	// 等待初始指纹建立
	time.Sleep(1200 * time.Millisecond)

	// 修改文件
	if err := os.WriteFile(file, []byte("llm:\n  temperature: 0.9\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case <-reloaded:
		// 成功
	case <-time.After(3 * time.Second):
		t.Fatal("修改配置后未触发热重载")
	}
}

// writeFullFixture 创建可被 Load 成功解析的完整配置
func writeFullFixture(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"main.yaml":                "server:\n  port: 1\n",
		"kernel.yaml":              "llm:\n  temperature: 0.1\n",
		"emotion.yaml":             "emotion:\n  dimensions: [开心]\n",
		"emotion_events.yaml":      "events:\n  default:\n    changes:\n      开心: 0.01\n",
		"memory_rag.yaml":          "rag:\n  embedding:\n    model: x\n  redis:\n    addr: 127.0.0.1:6379\n",
		"memory_block.yaml":        "memory_block:\n  max_entries: 10\n",
		"mcp.yaml":                 "mcp:\n  auto_start: false\n",
		"prompts/system_prompt.txt": "你是 Alice",
		"prompts/emotion_templates.yaml": "templates:\n  - name: calm\n    text: \"平静\"\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestWatchIgnoresUnchanged 未修改时不应重复回调
func TestWatchIgnoresUnchanged(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.yaml"), []byte("server:\n  port: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	count := 0
	Watch(dir, func(_ *Config) { count++ })

	time.Sleep(2200 * time.Millisecond)
	if count != 0 {
		t.Fatalf("文件未变化不应触发回调，实际 %d 次", count)
	}
}
