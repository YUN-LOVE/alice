package kernel

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"alice/config"
)

// fakeToolLLM 第一轮返回 tool_call，第二轮（含 tool 结果）返回纯文本
type fakeToolLLM struct{}

func (f *fakeToolLLM) Name() string { return "fake" }

func (f *fakeToolLLM) ChatStream(_ context.Context, messages []ChatMessage, _ []Tool) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 16)
	go func() {
		defer close(ch)
		hasTool := false
		for _, m := range messages {
			if m.Role == "tool" {
				hasTool = true
			}
		}
		if !hasTool {
			ch <- StreamChunk{ToolCalls: []ToolCall{{ID: "call_1", Name: "local__calculator", Arguments: `{"expression":"2*3"}`, Index: 0}}}
			ch <- StreamChunk{Done: true}
			return
		}
		// 验证 tool 结果已回传
		for _, m := range messages {
			if m.Role == "tool" && m.ToolCallID == "call_1" && strings.Contains(m.Content, "6") {
				for _, r := range []rune("结果是 6") {
					ch <- StreamChunk{Content: string(r)}
				}
			}
		}
		ch <- StreamChunk{Done: true}
	}()
	return ch, nil
}

// TestChatLoopFunctionCalling 验证：LLM tool_call → MCP 调用 → 结果回传 → 再生成 → 流式输出
func TestChatLoopFunctionCalling(t *testing.T) {
	cfg, err := config.Load("../../config")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	// 内置测试 MCP Server 二进制需已编译
	if !fileExists("../../mcp-server/alice-local-tools") {
		t.Skip("未编译内置 MCP Server，跳过")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	k := NewKernel(cfg)
	defer k.mcp.Stop("local")
	k.llm = &fakeToolLLM{}

	out, err := k.chatLoop(ctx, []ChatMessage{
		{Role: "system", Content: "你是 Alice"},
		{Role: "user", Content: "帮我计算 2*3"},
	})
	if err != nil {
		t.Fatalf("chatLoop 失败: %v", err)
	}

	var sb strings.Builder
	for c := range out {
		sb.WriteString(c.Content)
	}
	if sb.String() != "结果是 6" {
		t.Fatalf("最终输出错误: %q", sb.String())
	}
}

// TestChatLoopNoTool 无工具调用时直接返回内容
func TestChatLoopNoTool(t *testing.T) {
	cfg, err := config.Load("../../config")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	k := NewKernel(cfg)
	defer k.mcp.Stop("local")
	k.llm = &plainLLM{}

	out, err := k.chatLoop(ctx, []ChatMessage{
		{Role: "system", Content: "你是 Alice"},
		{Role: "user", Content: "你好"},
	})
	if err != nil {
		t.Fatalf("chatLoop 失败: %v", err)
	}
	var sb strings.Builder
	for c := range out {
		sb.WriteString(c.Content)
	}
	if sb.String() != "你好，我在" {
		t.Fatalf("输出错误: %q", sb.String())
	}
}

type plainLLM struct{}

func (p *plainLLM) Name() string { return "plain" }

func (p *plainLLM) ChatStream(_ context.Context, _ []ChatMessage, _ []Tool) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 4)
	go func() {
		defer close(ch)
		ch <- StreamChunk{Content: "你好，我在"}
		ch <- StreamChunk{Done: true}
	}()
	return ch, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// TestUserActiveRecently 启动即活跃（lastActive 已初始化），用户聊天时跳过主动推送
func TestUserActiveRecently(t *testing.T) {
	cfg, err := config.Load("../../config")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.Emotion.Proactive.SkipIfActiveMin <= 0 {
		t.Skip("skip_if_active_minutes 未配置，跳过")
	}

	k := NewKernel(cfg)
	defer k.mcp.Stop("local")

	// 刚启动（lastActive 初始化为 now），应判定为"用户活跃中"
	if !k.userActiveRecently() {
		t.Fatal("刚启动应判定为活跃，主动推送应被跳过")
	}

	// 模拟长时间无互动
	k.silentMu.Lock()
	k.lastActive = time.Now().Add(-time.Hour)
	k.silentMu.Unlock()
	if k.userActiveRecently() {
		t.Fatal("1 小时无互动不应判定为活跃")
	}
}
