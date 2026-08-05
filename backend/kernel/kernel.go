package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"alice/config"
	"alice/emotion"
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
	registry    *mcp.Registry
	onProactive func(string)

	silentMu       sync.Mutex
	lastActive     time.Time
	silentTriggered bool
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
		mcp:    mcp.NewManager(cfg.MCP.AutoStart, cfg.RAG.Redis.Addr, cfg.RAG.Redis.Password, cfg.RAG.Redis.DB),
		// 初始化活跃时间，避免启动时零值被误判为"长时间无互动"
		lastActive: time.Now(),
	}

	// 加载 MCP 市场注册表（本地文件或远程 URL）
	// 相对路径基于项目根（config 的上级）解析：config/ 与 registry/ 平级
	regBase := filepath.Dir(cfg.BaseDir)
	registryPath := resolveMCPCommand(regBase, cfg.MCP.Registry.File)
	if reg, err := mcp.LoadRegistry(cfg.MCP.Registry.Source, registryPath); err != nil {
		log.Printf("[kernel] 警告: MCP 注册表加载失败（市场不可用）: %v", err)
	} else {
		k.registry = reg
	}

	// 注册已配置的 MCP Server
	for _, s := range cfg.MCP.Servers {
		s2 := s
		if s2.Transport == "" {
			s2.Transport = "stdio"
		}
		s2.Command = resolveMCPCommand(cfg.BaseDir, s.Command)
		k.mcp.Add(s2.ID, s2.Name, s2)
	}
	// 启动已启用的 MCP Server（超时由 Client 内部控制）
	k.mcp.StartAll(context.Background())

	k.startEmotionTicker()
	k.startArchiveTicker()
	return k
}

// startArchiveTicker 每日零点整理：确认昨日对话已按日期归档（打 tag 存 RAG）
// 存储时已按日期打 tag，这里负责跨天边界确认与记录
func (k *Kernel) startArchiveTicker() {
	go func() {
		for {
			now := time.Now()
			// 下一个零点
			next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 1, 0, now.Location())
			time.Sleep(time.Until(next))

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			yes := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
			n, err := k.rag.CountByDate(ctx, yes)
			cancel()
			if err != nil {
				log.Printf("[archive] 零点整理失败: %v", err)
			} else {
				log.Printf("[archive] 零点整理完成：%s 共 %d 条对话已归档为长期记忆", yes, n)
			}
		}
	}()
}

// resolveMCPCommand 相对路径基于配置目录解析（避免受进程工作目录影响）；
// 纯命令名（如 npx / python）走系统 PATH，不做路径解析
func resolveMCPCommand(configDir, command string) string {
	if command == "" || filepath.IsAbs(command) || !strings.ContainsAny(command, `/\`) {
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
		if s2.Transport == "" {
			s2.Transport = "stdio"
		}
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

// startEmotionTicker 情绪时间演化 + 主动推送检测（LLM 生成 + 存记忆 + 广播）
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
			k.checkSilent()
			if should, _ := k.engine.ShouldProactive(); should {
				// 用户最近在聊天则不打扰（Alice 正在和你互动，不需要"想人"）
				if !k.userActiveRecently() {
					k.triggerProactive()
				}
			}
		}
	}()
}

// userActiveRecently 用户最近是否活跃过（活跃期跳过主动推送）
func (k *Kernel) userActiveRecently() bool {
	min := k.cfg.Emotion.Proactive.SkipIfActiveMin
	if min <= 0 {
		return false
	}
	k.silentMu.Lock()
	defer k.silentMu.Unlock()
	return time.Since(k.lastActive) < time.Duration(min)*time.Minute
}

// checkSilent 用户长时间无互动：触发 user_silent_long_time 事件（失落/焦虑上升）
func (k *Kernel) checkSilent() {
	min := k.cfg.Emotion.Proactive.SilentAfterMin
	if min <= 0 {
		return
	}
	k.silentMu.Lock()
	defer k.silentMu.Unlock()
	if time.Since(k.lastActive) > time.Duration(min)*time.Minute && !k.silentTriggered {
		k.silentTriggered = true
		k.engine.ProcessEvent("user_silent_long_time")
		log.Printf("[kernel] 用户长时间未互动，触发 user_silent_long_time | 状态: %v", roundMap(k.engine.State()))
	}
}

// triggerProactive 主动推送：LLM 生成话术 → 存入记忆 → 广播（异步）
func (k *Kernel) triggerProactive() {
	if !k.proactiveAllowedNow() {
		return
	}
	go func() {
		text := k.generateProactive()
		if text == "" {
			return
		}
		k.storeProactive(text)
		if k.onProactive != nil {
			k.onProactive(text)
		}
	}()
}

// proactiveAllowedNow 时段控制（避免深夜打扰）
func (k *Kernel) proactiveAllowedNow() bool {
	hours := k.cfg.Emotion.Proactive.Hours
	if len(hours) < 2 {
		return true
	}
	h := time.Now().Hour()
	start, end := hours[0], hours[1]
	if start <= end {
		return h >= start && h <= end
	}
	return h >= start || h <= end // 跨午夜时段
}

// generateProactive 由 LLM 根据当前情绪生成主动关心的话术（提示词模板在 prompts/proactive_prompt.txt）
func (k *Kernel) generateProactive() string {
	// mock 模式（未配 key）不做主动推送——复读式话术只会打扰用户
	if strings.Contains(k.llm.Name(), "(mock)") {
		log.Printf("[kernel] mock 模式，跳过主动推送")
		return ""
	}

	desc, _, _ := k.engine.Summary()
	prompt := k.cfg.Emotion.ProactivePrompt
	if prompt == "" {
		prompt = "你现在想主动联系用户说句话。{emotion} 请直接给出一句自然、真诚的关心或问候，一句话即可，不要解释。"
	}
	prompt = strings.ReplaceAll(prompt, "{emotion}", desc)

	messages := []ChatMessage{
		{Role: "system", Content: k.cfg.Kernel.SystemPrompt},
		{Role: "user", Content: prompt},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ch, err := k.llm.ChatStream(ctx, messages, nil)
	if err != nil {
		log.Printf("[kernel] 主动推送生成失败: %v", err)
		return ""
	}
	var sb strings.Builder
	for c := range ch {
		sb.WriteString(c.Content)
	}
	text := strings.TrimSpace(sb.String())

	// 兜底：输出复读了提示词原文（异常），丢弃
	if strings.Contains(text, "你现在想主动联系用户说句话") || strings.Contains(text, "{emotion}") {
		log.Printf("[kernel] 主动推送输出异常（复读提示词），丢弃: %s", truncateStr(text, 40))
		return ""
	}
	return text
}

// storeProactive 主动推送消息存入 RAG + Block（Alice 记得自己主动说过什么）
func (k *Kernel) storeProactive(text string) {
	today := time.Now().Format("2006-01-02")
	meta := map[string]any{"time": time.Now().Unix(), "date": today}
	if _, err := k.rag.Store(context.Background(), "assistant", text, meta); err != nil {
		log.Printf("[kernel] 主动推送存储失败: %v", err)
	}
	k.block.InjectIfAbsent(text, "assistant", "conversation")
	log.Printf("[kernel] 主动推送已生成并存入记忆: %s", truncateStr(text, 60))
}

// Process 处理用户消息，返回流式回复片段
func (k *Kernel) Process(ctx context.Context, sessionID, userText string) (<-chan StreamChunk, error) {
	_ = sessionID // 阶段三记忆/情绪全局共享

	// [1] 情绪引擎：事件识别 + 更新情绪向量 + 记录显著事件
	event := k.engine.DetectEvent(userText)
	k.engine.ProcessEvent(event)
	k.engine.RecordEvent(event)
	k.engine.Tick()
	log.Printf("[emotion] 事件=%s 状态=%v", event, roundMap(k.engine.State()))

	// 记录用户活跃时间（silent 检测用）
	k.silentMu.Lock()
	k.lastActive = time.Now()
	k.silentTriggered = false
	k.silentMu.Unlock()

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

			// 边接收边实时转发（保持真实流式节奏），同时聚合判断是否有工具调用。
			// 工具调用轮次 LLM 通常不输出文字；极少数"先说话再调工具"会残留已显示文字，可接受。
			var contentSB strings.Builder
			pending := map[int]ToolCall{}
			toolCallsSeen := false
			for c := range ch {
				if c.Content != "" {
					contentSB.WriteString(c.Content)
					if !toolCallsSeen {
						select {
						case <-ctx.Done():
							return
						case out <- StreamChunk{Content: c.Content}:
						}
					}
				}
				for _, tc := range c.ToolCalls {
					toolCallsSeen = true
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

			// 无工具调用：内容已实时转发完毕，本轮即最终回复
			if !toolCallsSeen {
				select {
				case <-ctx.Done():
					return
				case out <- StreamChunk{Done: true}:
				}
				k.store(ctx, lastUserText(messages), contentSB.String())
				return
			}

			toolCalls := make([]ToolCall, 0, len(pending))
			for i := 0; i < len(pending); i++ {
				if acc, ok := pending[i]; ok {
					toolCalls = append(toolCalls, acc)
				}
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

// store 回复完成后：本轮对话全量存入 RAG（按日期归档） + 回复注入 Block
func (k *Kernel) store(ctx context.Context, userText, reply string) {
	today := time.Now().Format("2006-01-02")
	meta := map[string]any{"time": time.Now().Unix(), "date": today}

	if _, err := k.rag.Store(ctx, "user", userText, meta); err != nil {
		log.Printf("[kernel] 存储用户消息失败: %v", err)
	}
	if reply != "" {
		if _, err := k.rag.Store(ctx, "assistant", reply, meta); err != nil {
			log.Printf("[kernel] 存储回复失败: %v", err)
		}
		k.block.InjectIfAbsent(reply, "assistant", "conversation")
	}

	// 情绪状态持久化（重启不失忆）
	ctx2, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := k.engine.Save(ctx2); err != nil {
		log.Printf("[emotion] 持久化失败: %v", err)
	}
}

// History 返回某日期的聊天记录（按时间升序），并注入 Memory Block 保证上下文连续
func (k *Kernel) History(ctx context.Context, date string) ([]memory.Mem, error) {
	mems, err := k.rag.RetrieveByDate(ctx, date)
	if err != nil {
		return nil, err
	}
	// 注入短期工作记忆（去重，Alice 记得当天聊过什么）
	entries := make([]memory.Entry, 0, len(mems))
	for _, m := range mems {
		entries = append(entries, memory.Entry{Role: m.Role, Text: m.Text, Source: "conversation"})
	}
	if n := k.block.Inject(entries); n > 0 {
		log.Printf("[kernel] 历史加载注入 Block %d 条（当前共 %d 条）", n, k.block.Len())
	}
	return mems, nil
}

// MemoryDates 返回已归档的日期列表
func (k *Kernel) MemoryDates(ctx context.Context) ([]string, error) { return k.rag.Dates(ctx) }

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

// EmotionEvents 返回最近的情绪显著事件
func (k *Kernel) EmotionEvents(ctx context.Context, limit int) ([]emotion.EmotionEventRecord, error) {
	return k.engine.Events(ctx, limit)
}

// MCP 相关：暴露给 server 层管理外设工具

// MCPStatus 返回所有 MCP Server 状态
func (k *Kernel) MCPStatus() []mcp.Status { return k.mcp.Status() }

// MCPStart 启动指定 MCP Server
func (k *Kernel) MCPStart(ctx context.Context, id string) error { return k.mcp.Start(ctx, id) }

// MCPStop 停止指定 MCP Server
func (k *Kernel) MCPStop(id string) { k.mcp.Stop(id) }

// MCPToolToggle 工具级启用/禁用
func (k *Kernel) MCPToolToggle(serverID, toolName string, enabled bool) error {
	return k.mcp.SetToolEnabled(serverID, toolName, enabled)
}

// ProactiveEnabled 主动推送开关状态
func (k *Kernel) ProactiveEnabled() bool { return k.engine.ProactiveEnabled() }

// SetProactiveEnabled 切换主动推送开关（运行时，持久化）
func (k *Kernel) SetProactiveEnabled(enabled bool) {
	k.engine.SetProactiveEnabled(enabled)
	log.Printf("[kernel] 主动推送已%s", map[bool]string{true: "开启", false: "关闭"}[enabled])
}

// MCPRegistry 返回 MCP 市场注册表
func (k *Kernel) MCPRegistry() *mcp.Registry {
	return k.registry
}

// MCPInstall 安装注册表中的 MCP：写入 mcp.yaml 并触发热重载
func (k *Kernel) MCPInstall(id string) error {
	if k.registry == nil {
		return fmt.Errorf("注册表未加载")
	}
	item, ok := k.registry.Item(id)
	if !ok {
		return fmt.Errorf("注册表中不存在: %s", id)
	}
	// 已存在则忽略
	for _, s := range k.cfg.MCP.Servers {
		if s.ID == id {
			return nil
		}
	}
	servers := append(k.cfg.MCP.Servers, item.ToConfig())
	if err := config.UpdateMCP(k.cfg.BaseDir, servers); err != nil {
		return err
	}
	log.Printf("[mcp] 已安装: %s", id)
	return nil
}

// MCPUninstall 卸载 MCP：从 mcp.yaml 移除并触发热重载
func (k *Kernel) MCPUninstall(id string) error {
	servers := make([]config.MCPServerConfig, 0, len(k.cfg.MCP.Servers))
	removed := false
	for _, s := range k.cfg.MCP.Servers {
		if s.ID == id {
			removed = true
			continue
		}
		servers = append(servers, s)
	}
	if !removed {
		return fmt.Errorf("未安装: %s", id)
	}
	if err := config.UpdateMCP(k.cfg.BaseDir, servers); err != nil {
		return err
	}
	log.Printf("[mcp] 已卸载: %s", id)
	return nil
}
