package kernel

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"alice/config"
	"alice/emotion"
	"alice/memory"
)

// Kernel 对话核心：
// 阶段三链路 = Emotion 事件驱动 → RAG 检索 → Memory Block 增量注入 → 构建上下文（含情绪） → LLM → 存储
//
// 记忆模型：
// - Memory Block：Alice 的短期工作记忆，全局共享（存 RAG 检索原文 + 近期对话，去重、只读）
// - RAG：长期记忆，每轮对话全量存储，检索结果经 Block 注入上下文
// - Emotion：高维情绪向量，事件驱动 + 时间衰减，注入回复风格，超阈值触发主动推送
// - sessionID 保留在协议层（多端各自持有），阶段三记忆/情绪全局一致
type Kernel struct {
	cfg     *config.Config
	llm     LLMClient
	rag     *memory.RAG
	block   *memory.Block
	engine  *emotion.Engine
	onProactive func(string)
}

// NewKernel 创建 Kernel
func NewKernel(cfg *config.Config) *Kernel {
	embedder := memory.NewEmbedder(
		cfg.RAG.Embedding.Provider,
		cfg.RAG.Embedding.BaseURL,
		cfg.RAG.Embedding.APIKey,
		cfg.RAG.Embedding.Model,
	)

	// hash 兜底模式（未配 Key）下向量无语义精度，相似度普遍偏低；
	// 不做绝对过滤，仅靠 topK 排序截断，保证开发链路可测
	minScore := cfg.RAG.Retrieval.MinScore
	if _, isHash := embedder.(*memory.HashEmbedder); isHash {
		minScore = 0
	}

	rag := memory.NewRAG(
		cfg.RAG.Redis.Addr,
		cfg.RAG.Redis.Password,
		cfg.RAG.Redis.DB,
		embedder,
		cfg.RAG.Retrieval.TopK,
		minScore,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rag.Ping(ctx); err != nil {
		log.Printf("[kernel] 警告: Redis 不可用，记忆系统降级（对话仍可用）: %v", err)
	}

	k := &Kernel{
		cfg:   cfg,
		llm:   NewLLMClient(cfg.Kernel.LLM.Provider, cfg.Kernel.LLM.BaseURL, cfg.Kernel.LLM.APIKey, cfg.Kernel.LLM.Model, cfg.Kernel.LLM.Temperature, cfg.Kernel.LLM.MaxTokens),
		rag:   rag,
		block: memory.NewBlock(cfg.Block.MaxEntries),
		engine: emotion.New(cfg.Emotion,
			cfg.RAG.Redis.Addr,
			cfg.RAG.Redis.Password,
			cfg.RAG.Redis.DB,
		),
	}
	k.startEmotionTicker()
	return k
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

	// [5] LLM 生成回复
	ch, err := k.llm.ChatStream(ctx, messages, nil)
	if err != nil {
		return nil, err
	}

	// [6] 流式转发 + 回复完成后存储
	out := make(chan StreamChunk, 64)
	go func() {
		defer close(out)
		var sb strings.Builder
		for c := range ch {
			if c.Content != "" {
				sb.WriteString(c.Content)
			}
			out <- c
			if c.Done {
				k.store(ctx, userText, sb.String())
			}
		}
	}()

	return out, nil
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
