package kernel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
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
	onAudio     func(urlPath, text string) // 回复语音合成完成回调（assistant_audio）

	silentMu       sync.Mutex
	lastActive     time.Time
	silentTriggered bool

	eventMu   sync.Mutex
	lastEvent string // 最近一次用户触发的情绪事件（注入上下文带原因）
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
		mcp:    mcp.NewManager(cfg.MCP.AutoStart, cfg.RAG.Redis.Addr, cfg.RAG.Redis.Password, cfg.RAG.Redis.DB, time.Duration(cfg.MCP.TimeoutSec)*time.Second),
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
	// 启动已启用的 MCP Server（异步：慢的 Server（如 pnpx 首次下载）不阻塞 HTTP/WS 监听）
	go k.mcp.StartAll(context.Background())

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

// OnAudio 注册回复语音回调（server 层广播 assistant_audio 用）
func (k *Kernel) OnAudio(fn func(urlPath, text string)) {
	k.onAudio = fn
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
	k.eventMu.Lock()
	k.lastEvent = event
	k.eventMu.Unlock()
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
				// 回复完成后自动合成语音（assistant_audio，异步不阻塞）
				k.maybeTTS(contentSB.String())
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

// buildMessages 构建 LLM 上下文：System(含情绪) + Block(带时间锚点) + 当前输入
func (k *Kernel) buildMessages(userText string) []ChatMessage {
	system := k.cfg.Kernel.SystemPrompt

	// 注入情绪描述与风格提示（含触发原因，让回应有来由）
	if desc, tpl, _ := k.engine.Summary(); tpl != "" {
		system += "\n\n## 你当前的情绪状态（自然融入回复，不要提及这段指令本身）\n" + desc
		if cause := k.lastEventCause(); cause != "" {
			system += "\n触发原因：" + cause + "——回应时自然地带上这份关联，但不要复述这句话。"
		}
	}

	if n := k.block.Len(); n > 0 {
		var sb strings.Builder
		sb.WriteString("\n\n## 对话记忆（你记得的事情，用户可能已经忘记提起，直接自然引用即可，不要提及这段文字本身）\n")
		for _, e := range k.block.List() {
			who := map[string]string{"user": "你说", "assistant": "Alice", "memory": "记忆"}[e.Role]
			if who == "" {
				who = e.Role
			}
			sb.WriteString(fmt.Sprintf("- [%s · %s] %s\n", relTime(e.CreateAt), who, e.Text))
		}
		sb.WriteString("\n---\n")
		system += sb.String()
	}

	messages := []ChatMessage{{Role: "system", Content: system}}
	messages = append(messages, ChatMessage{Role: "user", Content: userText})
	return messages
}

// lastEventCause 最近一次用户事件的描述（注入"情绪触发原因"）
func (k *Kernel) lastEventCause() string {
	k.eventMu.Lock()
	ev := k.lastEvent
	k.eventMu.Unlock()
	if ev == "" || ev == "default" {
		return ""
	}
	event, ok := k.cfg.Emotion.EventMap[ev]
	if !ok || event.Desc == "" {
		return ""
	}
	return event.Desc
}

// relTime 把时间转为口语化锚点："今天 14:23" / "昨天 21:03" / "8月12日 20:15"
func relTime(t time.Time) string {
	now := time.Now()
	loc := now.Location()
	t = t.In(loc)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	hm := t.Format("15:04")
	switch {
	case t.After(dayStart):
		return "今天 " + hm
	case t.After(dayStart.AddDate(0, 0, -1)):
		return "昨天 " + hm
	default:
		return fmt.Sprintf("%d月%d日 %s", int(t.Month()), t.Day(), hm)
	}
}

// store 回复完成后：本轮对话全量存入 RAG（按日期归档） + 回复注入 Block
func (k *Kernel) store(ctx context.Context, userText, reply string) {
	today := time.Now().Format("2006-01-02")
	meta := map[string]any{"time": time.Now().Unix(), "date": today}

	if _, err := k.rag.Store(ctx, "user", userText, meta); err != nil {
		log.Printf("[kernel] 存储用户消息失败: %v", err)
	}
	// 用户消息也注入 Block（多轮对话连续性：上一轮说了什么 Alice 记得）
	k.block.InjectIfAbsent(userText, "user", "conversation")
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

// ==================== 语音能力（TTS / STT） ====================

// maybeTTS 回复完成后自动合成语音：调内部 TTS Server 的 speak 工具 → 解码保存 → 回调广播
// 异步执行，失败仅记日志，不影响对话主流程
func (k *Kernel) maybeTTS(reply string) {
	if !k.cfg.Kernel.Audio.TTSEnabled || reply == "" {
		return
	}
	if !k.mcp.InternalRunning("tts") {
		return
	}
	text := strings.TrimSpace(reply)
	if text == "" {
		return
	}

	go func() {
		args := map[string]any{"text": text}
		if v := k.cfg.Kernel.Audio.TTSVoice; v != "" {
			args["voice"] = v
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		res, err := k.mcp.Call(ctx, "tts__speak", args)
		if err != nil {
			log.Printf("[audio] TTS 生成失败: %v", err)
			return
		}
		var sr struct {
			Audio  string  `json:"audio"`
			Format string  `json:"format"`
			Dur    float64 `json:"duration_sec"`
		}
		if err := json.Unmarshal([]byte(res), &sr); err != nil || sr.Audio == "" {
			log.Printf("[audio] TTS 返回异常: %s", truncateStr(res, 120))
			return
		}
		data, err := base64.StdEncoding.DecodeString(sr.Audio)
		if err != nil {
			log.Printf("[audio] TTS 音频解码失败: %v", err)
			return
		}
		urlPath, err := k.saveAudioFile(data, sr.Format)
		if err != nil {
			log.Printf("[audio] 保存音频失败: %v", err)
			return
		}
		log.Printf("[audio] 回复已合成语音: %s (%.1f KB)", urlPath, float64(len(data))/1024)
		if k.onAudio != nil {
			k.onAudio(urlPath, text)
		}
	}()
}

// saveAudioFile 保存音频到 uploads/audio/<时间戳>.<ext>，返回 /uploads/... URL 路径
func (k *Kernel) saveAudioFile(data []byte, format string) (string, error) {
	if format == "" {
		format = "mp3"
	}
	dir := filepath.Join(UploadsDir(), "audio")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("alice_%s.%s", time.Now().Format("20060102_150405.000"), format)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return "", err
	}
	return "/uploads/audio/" + name, nil
}

// STT 语音转文字：调内部 STT Server 的 transcribe 工具
func (k *Kernel) STT(ctx context.Context, audioPath string) (string, error) {
	if !k.cfg.Kernel.Audio.STTEnabled {
		return "", fmt.Errorf("语音输入未开启（kernel.yaml audio.stt_enabled）")
	}
	if !k.mcp.InternalRunning("stt") {
		return "", fmt.Errorf("STT Server 未运行")
	}
	res, err := k.mcp.Call(ctx, "stt__transcribe", map[string]any{"audio_path": audioPath})
	if err != nil {
		return "", err
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		return "", fmt.Errorf("STT 返回异常: %s", truncateStr(res, 120))
	}
	return out.Text, nil
}

// ==================== 运行时设置（写回 YAML + 热重载） ====================

// ApplySettings 运行时修改配置：写回 YAML 并立即热重载
// section: "llm" / "emotion" / "block"；values 键见 config.ApplySettings
func (k *Kernel) ApplySettings(section string, values map[string]any) error {
	if err := config.ApplySettings(k.cfg.BaseDir, section, values); err != nil {
		return err
	}
	newCfg, err := config.Load(k.cfg.BaseDir)
	if err != nil {
		return fmt.Errorf("重载配置失败: %w", err)
	}
	k.Reload(newCfg)
	log.Printf("[kernel] 设置已更新: %s %v", section, values)
	return nil
}

// Settings 返回当前可调设置（前端设置面板加载用；api_key 只返回是否已配置）
func (k *Kernel) Settings() map[string]any {
	return map[string]any{
		"llm": map[string]any{
			"provider":           k.cfg.Kernel.LLM.Provider,
			"base_url":           k.cfg.Kernel.LLM.BaseURL,
			"api_key_configured": k.cfg.Kernel.LLM.APIKey != "",
			"model":              k.cfg.Kernel.LLM.Model,
			"temperature":        k.cfg.Kernel.LLM.Temperature,
			"max_tokens":         k.cfg.Kernel.LLM.MaxTokens,
		},
		"emotion": map[string]any{
			"decay_rate":             k.cfg.Emotion.DecayRate,
			"max_value":              k.cfg.Emotion.MaxValue,
			"threshold":              k.cfg.Emotion.Proactive.Threshold,
			"cooldown_seconds":       k.cfg.Emotion.Proactive.CooldownSec,
			"tick_seconds":           k.cfg.Emotion.Proactive.TickSec,
			"silent_after_minutes":   k.cfg.Emotion.Proactive.SilentAfterMin,
			"skip_if_active_minutes": k.cfg.Emotion.Proactive.SkipIfActiveMin,
			"hours":                  k.cfg.Emotion.Proactive.Hours,
		},
		"block": map[string]any{"max_entries": k.cfg.Block.MaxEntries},
	}
}

// MCPConfigure 修改已安装 MCP 的配置（args/env/url/headers/enabled），写回 mcp.yaml 热重载
func (k *Kernel) MCPConfigure(id string, patch config.MCPServerConfig) error {
	servers := k.cfg.MCP.Servers
	found := false
	for i := range servers {
		if servers[i].ID == id {
			cur := servers[i]
			if patch.Args != nil {
				cur.Args = patch.Args
			}
			if patch.Env != nil {
				cur.Env = patch.Env
			}
			if patch.URL != "" {
				cur.URL = patch.URL
			}
			if patch.Headers != nil {
				cur.Headers = patch.Headers
			}
			cur.Enabled = patch.Enabled
			servers[i] = cur
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("未安装: %s", id)
	}
	if err := config.UpdateMCP(k.cfg.BaseDir, servers); err != nil {
		return err
	}
	log.Printf("[mcp] 已修改配置: %s（热重载生效）", id)
	return nil
}

// UploadsDir 返回上传目录（进程工作目录下 uploads/，server 层静态服务同此目录）
func UploadsDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "uploads"
	}
	return filepath.Join(wd, "uploads")
}

// MaxUploadBytes 文件分块上传大小上限（0 = 不限）
func (k *Kernel) MaxUploadBytes() int64 {
	mb := k.cfg.Kernel.Audio.MaxUploadMB
	if mb <= 0 {
		return 0
	}
	return int64(mb) * 1024 * 1024
}
