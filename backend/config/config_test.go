package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadAllSections 确保所有 YAML 顶层命名空间都能正确解析
// 历史教训：结构体顶层 tag 缺失时 yaml.Unmarshal 静默忽略，字段全部为空
func TestLoadAllSections(t *testing.T) {
	dir, err := os.MkdirTemp("", "alice-config-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	files := map[string]string{
		"main.yaml": `
server:
  host: "0.0.0.0"
  port: 9999
  ws_path: "/ws"
log_level: "debug"
`,
		"kernel.yaml": `
llm:
  provider: "deepseek"
  base_url: "https://api.deepseek.com/v1"
  api_key: "sk-test"
  model: "deepseek-chat"
context:
  max_messages: 42
`,
		"emotion.yaml": `
emotion:
  dimensions: [开心]
  decay_rate: 0.002
`,
		"emotion_events.yaml": `
events:
  user_greeting:
    开心: 0.05
`,
		"memory_rag.yaml": `
rag:
  embedding:
    provider: "siliconflow"
    base_url: "https://api.siliconflow.cn/v1"
    api_key: "sk-embed"
    model: "BAAI/bge-m3"
  redis:
    addr: "1.2.3.4:6379"
    db: 3
  retrieval:
    top_k: 7
    min_score: 0.25
`,
		"memory_block.yaml": `
memory_block:
  max_entries: 88
`,
		"mcp.yaml": `
mcp:
  auto_start: true
`,
		"prompts/system_prompt.txt": "你是 Alice",
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

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"main.port", cfg.Main.Server.Port, 9999},
		{"main.ws_path", cfg.Main.Server.WSPath, "/ws"},
		{"kernel.llm.model", cfg.Kernel.LLM.Model, "deepseek-chat"},
		{"kernel.context.max", cfg.Kernel.Context.MaxMessages, 42},
		{"emotion.dimensions", cfg.Emotion.Dimensions, []string{"开心"}},
		{"rag.embedding.model", cfg.RAG.Embedding.Model, "BAAI/bge-m3"},
		{"rag.redis.addr", cfg.RAG.Redis.Addr, "1.2.3.4:6379"},
		{"rag.retrieval.top_k", cfg.RAG.Retrieval.TopK, 7},
		{"block.max_entries", cfg.Block.MaxEntries, 88},
		{"mcp.auto_start", cfg.MCP.AutoStart, true},
		{"system_prompt", cfg.Kernel.SystemPrompt, "你是 Alice"},
	}

	for _, tt := range tests {
		switch want := tt.want.(type) {
		case int:
			if got := tt.got.(int); got != want {
				t.Errorf("%s = %d, want %d", tt.name, got, want)
			}
		case string:
			if got := tt.got.(string); got != want {
				t.Errorf("%s = %q, want %q", tt.name, got, want)
			}
		case bool:
			if got := tt.got.(bool); got != want {
				t.Errorf("%s = %v, want %v", tt.name, got, want)
			}
		}
	}

	if len(cfg.Emotion.Dimensions) != 1 || cfg.Emotion.Dimensions[0] != "开心" {
		t.Errorf("emotion.dimensions 解析错误: %v", cfg.Emotion.Dimensions)
	}
}
