package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"alice/config"
	"alice/emotion"
	"path/filepath"
	"alice/mcp"
	"alice/memory"
)

// Kernel 对话核心：
// 阶段四链路 = Emotion → RAG 检索 → Memory Block 注入 → 构建上下文 → LLM（Function Calling 循环，经 MCP 调工具） → 存储
//
// 记忆模型：
// - Memory Block：Alice 的短期工作记忆，全局共享（存 RAG 检索原文 + 近期对话，去重、只读）
// - RAG：长期记忆，每轮对话全量存储，检索结果经 Block 注入上下文
// - Emotion：高维情绪向量，事件驱动 + 时间衰减，注入回复风格，超阈值触发主动推送
// - MCP：外设工具统一接入，LLM 通过 Function Calling 自动决策调用
type Kernel struct {
	cfg         *config.Config
	llm         LLMClient
	rag         *memory.RAG
	block       *memory.Block
	engine      *emotion.Engine
	mcp         *mcp.Manager
	onProactive func(string)
}

// NewKernel 创建 Kernel
func NewKernel(cfg *config.Config) *Kernel {
	rag := buildRAG(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rag.Ping(ctx); err != nil {
		log.Printf("[kernel] 警告: Redis 不可用，记忆系统降级（对话仍可用）: %v", err)
	}

	k := &Kernel{
		cfg:    cfg,
		llm:    buildLLM(cfg),
		rag:    rag,
		block:  memory.NewBlock(cfg.Block.MaxEntries),
		engine: buildEmotion(cfg),
		mcp:    mcp.NewManager(cfg.MCP.AutoStart),
	}

	// 注册已配置的 MCP Server
	for _, s := range cfg.MCP.Servers {
		k.mcp.Add(s.ID, s.Name, resolveMCPCommand(cfg.BaseDir, s.Command), s.Args, s.Env, s.Enabled)
	}
	// 启动已启用的 MCP Server（超时由 Client 内部控制）
	k.mcp.StartAll(context.Background())

	k.startEmotionTicker()
	return k
}

// resolveMCPCommand 相对路径基于配置目录解析（避免受进程工作目录影响）
func resolveMCPCommand(configDir, command string) string {
	if command == "" || filepath.IsAbs(command) {
		return command
	}
	abs, err := filepath.Abs(filepath.Join(configDir, command))
	if err != nil {
		return command
	}
	return abs
}

// Reload 配置热重载：更新 LLM / 情绪引擎 / RAG 检索参数 / Memory Block 容量 / MCP Server
// 保留 Redis 连接与已存记忆；短期工作记忆内容保留（仅调整容量）
func (k *Kernel) Reload(cfg *config.Config) {
	// MCP 配置变更时热重载（需在更新 k.cfg 前比较新旧配置）
	k.reloadMCP(k.cfg.MCP, cfg.MCP, cfg.BaseDir)

	k.cfg = cfg
	k.llm = buildLLM(cfg)
	k.engine = buildEmotion(cfg)
	k.rag.Reconfigure(
		buildEmbedder(cfg),
		cfg.RAG.Retrieval.TopK,
		ragMinScore(cfg),
	)
	k.block.SetMaxEntries(cfg.Block.MaxEntries)

	log.Printf("[kernel] 配置已热重载 | LLM: %s | 情绪恢复: %v", k.llm.Name(), k.engine.State())
}

// reloadMCP 仅当 MCP 配置变化时重建 Manager
func (k *Kernel) reloadMCP(old, cur *config.MCPConfig, baseDir string) {
	if mcpConfigEqual(old.Servers, cur.Servers) {
		return
	}
	servers := make([]config.MCPServerConfig, len(cur.Servers))
	for i, s := range cur.Servers {
		s2 := s
		s2.Command = resolveMCPCommand(baseDir, s.Command)
		servers[i] = s2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	k.mcp.Reload(ctx, servers)
}

// mcpConfigEqual 比较两份 MCP Server 配置是否相同
func mcpConfigEqual(a, b []config.MCPServerConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Command != b[i].Command ||
			a[i].Name != b[i].Name || a[i].Enabled != b[i].Enabled {
			return false
		}
	}
	return true
}

func buildLLM(cfg *config.Config) LLMClient {
	return NewLLMClient(
		cfg.Kernel.LLM.Provider, cfg.Kernel.LLM.BaseURL, cfg.Kernel.LLM.APIKey,
		cfg.Kernel.LLM.Model, cfg.Kernel.LLM.Temperature, cfg.Kernel.LLM.MaxTokens,
	)
}

func buildEmotion(cfg *config.Config) *emotion.Engine {
	return emotion.New(cfg.Emotion, cfg.RAG.Redis.Addr, cfg.RAG.Redis.Password, cfg.RAG.Redis.DB)
}

func buildEmbedder(cfg *config.Config) memory.Embedder {
	return memory.NewEmbedder(
		cfg.RAG.Embedding.Provider,
		cfg.RAG.Embedding.BaseURL,
		cfg.RAG.Embedding.APIKey,
		cfg.RAG.Embedding.Model,
	)
}

// ragMinScore hash 兜底模式（未配 Key）下向量无语义精度，相似度普遍偏低；
// 不做绝对过滤，仅靠 topK 排序截断，保证开发链路可测
func ragMinScore(cfg *config.Config) float64 {
	if _, isHash := buildEmbedder(cfg).(*memory.HashEmbedder); isHash {
		return 0
	}
	return cfg.RAG.Retrieval.MinScore
}

func buildRAG(cfg *config.Config) *memory.RAG {
	return memory.NewRAG(
		cfg.RAG.Redis.Addr,
		cfg.RAG.Redis.Password,
		cfg.RAG.Redis.DB,
		buildEmbedder(cfg),
		cfg.RAG.Retrieval.TopK,
		ragMinScore(cfg),
	)
}

// OnProactive 注册主动推送回调（server 层广播用）
func (k *Kernel) OnProactive(fn func(text string)) {
	k.onProactive = fn
}

// Emotion 暴露情绪引擎（server 层推 emotion_update 用）
func (k *Kernel) Emotion() *emotion.Engine { return k.engine }

// startEmotionTicker 情绪时间演化 + 主动推送检测
func (k *Kernel) startEmotionTicker() {
	tickSec := k.cfg.Emotion.Proactive.TickSec
	if tickSec <= 0 {
		tickSec = 1
	}
	go func() {
		ticker := time.NewTicker(time.Duration(tickSec) * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			k.engine.Tick()
			if k.onProactive != nil {
				if should, text := k.engine.ShouldProactive(); should {
					log.Printf("[kernel] 主动推送: %s", text)
					k.onProactive(text)
				}
			}
		}
	}()
}

// Process 处理用户消息，返回流式回复片段
func (k *Kernel) Process(ctx context.Context, sessionID, userText string) (<-chan StreamChunk, error) {
	_ = sessionID // 阶段三记忆/情绪全局共享

	// [1] 情绪引擎：事件识别 + 更新情绪向量
	event := k.engine.DetectEvent(userText)
	k.engine.ProcessEvent(event)
	k.engine.Tick()
	log.Printf("[emotion] 事件=%s 状态=%v", event, roundMap(k.engine.State()))

	// [2] RAG 检索相关历史记忆
	memories := k.retrieve(ctx, userText)

	// [3] Memory Block 增量注入检索结果（原文，去重）
	k.injectMemories(memories)

	// [4] 构建上下文：System(含情绪) + Memory Block + 用户输入
	messages := k.buildMessages(userText)

	// [5] LLM 生成回复（Function Calling 循环，经 MCP 调用外设工具）
	out, err := k.chatLoop(ctx, messages)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// chatLoop Function Calling 循环：
// LLM 流式输出 → 若含 tool_calls 则执行 MCP 工具 → 结果回传 → 再生成，直到返回纯文本
func (k *Kernel) chatLoop(ctx context.Context, messages []ChatMessage) (<-chan StreamChunk, error) {
	out := make(chan StreamChunk, 64)

	go func() {
		defer close(out)

		for {
			tools := mcpTools(k.mcp.Tools())
			ch, err := k.llm.ChatStream(ctx, messages, tools)
			if err != nil {
				log.Printf("[kernel] LLM 调用失败: %v", err)
				out <- StreamChunk{Done: true}
				return
			}

			// 聚合本轮输出（content + tool_calls 分片）
			var contentSB strings.Builder
			pending := map[int]ToolCall{}
			for c := range ch {
				if c.Content != "" {
					contentSB.WriteString(c.Content)
				}
				for _, tc := range c.ToolCalls {
					acc := pending[tc.Index]
					if tc.ID != "" {
						acc.ID = tc.ID
					}
					if tc.Name != "" {
						acc.Name = tc.Name
					}
					acc.Arguments += tc.Arguments
					acc.Index = tc.Index
					pending[tc.Index] = acc
				}
			}

			toolCalls := make([]ToolCall, 0, len(pending))
			for i := 0; i < len(pending); i++ {
				if acc, ok := pending[i]; ok {
					toolCalls = append(toolCalls, acc)
				}
			}

			// 无工具调用：本轮即为最终回复，转发内容
			if len(toolCalls) == 0 {
				content := contentSB.String()
				for _, r := range []rune(content) {
					select {
					case <-ctx.Done():
						return
					case out <- StreamChunk{Content: string(r)}:
					}
				}
				select {
				case <-ctx.Done():
				case out <- StreamChunk{Done: true}:
				}
				k.store(ctx, lastUserText(messages), content)
				return
			}

			// 有工具调用：执行并回传
			messages = append(messages, assistantToolMessage(toolCalls))
			for _, tc := range toolCalls {
				args := parseToolArgs(tc.Arguments)
				result, err := k.mcp.Call(ctx, tc.Name, args)
				if err != nil {
					result = "工具调用失败: " + err.Error()
				}
				log.Printf("[mcp] 调用 %s → %s", tc.Name, truncateStr(result, 120))
				messages = append(messages, ChatMessage{Role: "tool", ToolCallID: tc.ID, Content: result})
			}
			// 循环：带工具结果再生成
		}
	}()

	return out, nil
}

// mcpTools MCP 工具 → LLM function schema
func mcpTools(mcps []mcp.Tool) []Tool {
	out := make([]Tool, 0, len(mcps))
	for _, t := range mcps {
		params := t.InputSchema
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, Tool{
			Type: "function",
			Function: ToolDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return out
}

// assistantToolMessage 构造 assistant 消息（携带 tool_calls，OpenAI 格式）
func assistantToolMessage(toolCalls []ToolCall) ChatMessage {
	calls := make([]any, 0, len(toolCalls))
	for _, tc := range toolCalls {
		calls = append(calls, map[string]any{
			"id":   tc.ID,
			"type": "function",
			"function": map[string]any{
				"name":      tc.Name,
				"arguments": tc.Arguments,
			},
		})
	}
	return ChatMessage{Role: "assistant", Content: "", ToolCalls: calls}
}

// parseToolArgs 解析工具调用参数 JSON
func parseToolArgs(args string) map[string]any {
	var out map[string]any
	if err := json.Unmarshal([]byte(args), &out); err != nil {
		return map[string]any{}
	}
	return out
}

// lastUserText 获取 messages 中最后一条 user 消息（用于记忆存储）
func lastUserText(messages []ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

// retrieve 检索记忆；失败时降级返回空（对话不中断）
func (k *Kernel) retrieve(ctx context.Context, query string) []memory.SearchResult {
	results, err := k.rag.Retrieve(ctx, query)
	if err != nil {
		log.Printf("[kernel] RAG 检索失败（降级）: %v", err)
		return nil
	}
	if len(results) > 0 {
		log.Printf("[kernel] RAG 命中 %d 条记忆", len(results))
	}
	return results
}

// injectMemories 把检索结果注入 Memory Block
func (k *Kernel) injectMemories(results []memory.SearchResult) {
	entries := make([]memory.Entry, 0, len(results))
	for _, r := range results {
		entries = append(entries, memory.Entry{
			Role:   r.Mem.Role,
			Text:   r.Mem.Text,
			Source: "rag",
		})
	}
	injected := k.block.Inject(entries)
	if injected > 0 {
		log.Printf("[kernel] Memory Block 注入 %d 条记忆（当前共 %d 条）", injected, k.block.Len())
	}
}

// buildMessages 构建 LLM 上下文：System(含情绪) + Block + 当前输入
func (k *Kernel) buildMessages(userText string) []ChatMessage {
	system := k.cfg.Kernel.SystemPrompt

	// 注入情绪描述与风格提示
	if desc, tpl, _ := k.engine.Summary(); tpl != "" {
		system += "\n\n## 你当前的情绪状态（自然融入回复，不要提及这段指令本身）\n" + desc
	}

	if n := k.block.Len(); n > 0 {
		var sb strings.Builder
		sb.WriteString("\n\n## 对话记忆（你记得的事情，用户可能已经忘记提起，直接自然引用即可，不要提及这段文字本身）\n")
		for _, e := range k.block.List() {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", e.Role, e.Text))
		}
		sb.WriteString("\n---\n")
		system += sb.String()
	}

	messages := []ChatMessage{{Role: "system", Content: system}}
	messages = append(messages, ChatMessage{Role: "user", Content: userText})
	return messages
}

// store 回复完成后：本轮对话全量存入 RAG + 回复注入 Block
func (k *Kernel) store(ctx context.Context, userText, reply string) {
	meta := map[string]any{"time": time.Now().Unix()}

	if _, err := k.rag.Store(ctx, "user", userText, meta); err != nil {
		log.Printf("[kernel] 存储用户消息失败: %v", err)
	}
	if _, err := k.rag.Store(ctx, "assistant", reply, meta); err != nil {
		log.Printf("[kernel] 存储回复失败: %v", err)
	}
	if reply != "" {
		k.block.InjectIfAbsent(reply, "assistant", "conversation")
	}

	// 情绪状态持久化（重启不失忆）
	ctx2, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := k.engine.Save(ctx2); err != nil {
		log.Printf("[emotion] 持久化失败: %v", err)
	}
}

// roundMap 情绪向量取整，便于日志输出
func roundMap(m map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = float64(int(v*1000+0.5)) / 1000
	}
	return out
}

// BlockList 返回 Memory Block 内容（前端只读）
func (k *Kernel) BlockList() []memory.Entry { return k.block.List() }

// BlockGet 返回单条记忆
func (k *Kernel) BlockGet(id int64) (memory.Entry, bool) { return k.block.Get(id) }

// MemorySearch 搜索 RAG 历史
func (k *Kernel) MemorySearch(ctx context.Context, query string) ([]memory.SearchResult, error) {
	return k.rag.Retrieve(ctx, query)
}

// MemoryExport 导出全部记忆
func (k *Kernel) MemoryExport(ctx context.Context) ([]memory.Mem, error) { return k.rag.All(ctx) }

// MemoryCount 记忆总数
func (k *Kernel) MemoryCount(ctx context.Context) (int64, error) { return k.rag.Count(ctx) }

// Reset 清空会话（清空 Memory Block，记忆 RAG 保留——保证 Alice 人设连续性）
func (k *Kernel) Reset(sessionID string) {
	_ = sessionID
	k.block.Clear()
	log.Printf("[kernel] 已清空 Memory Block")
}

// LLMName 返回当前 LLM 名称（用于握手信息）
func (k *Kernel) LLMName() string { return k.llm.Name() }

// EmbedderName 返回当前 Embedding 名称
func (k *Kernel) EmbedderName() string { return k.rag.EmbedderName() }

// MCP 相关：暴露给 server 层管理外设工具

// MCPStatus 返回所有 MCP Server 状态
func (k *Kernel) MCPStatus() []mcp.Status { return k.mcp.Status() }

// MCPStart 启动指定 MCP Server
func (k *Kernel) MCPStart(ctx context.Context, id string) error { return k.mcp.Start(ctx, id) }

// MCPStop 停止指定 MCP Server
func (k *Kernel) MCPStop(id string) { k.mcp.Stop(id) }
