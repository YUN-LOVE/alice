package memory

import (
	"sync"
	"time"
)

// Entry Memory Block 中的一条记忆
type Entry struct {
	ID       int64     `json:"id"`
	Role     string    `json:"role"` // user / assistant / memory
	Text     string    `json:"text"` // 原文，不压缩不摘要
	Source   string    `json:"source"`
	CreateAt time.Time `json:"create_at"`
}

// Block 短期工作记忆：
// - 存 RAG 查询结果的原文 + 近期对话
// - 增量注入，只加没有的（按文本去重）
// - 直接注入上下文，LLM 自然引用
// - 前端只读
type Block struct {
	mu         sync.RWMutex
	entries    []Entry
	maxEntries int // 0 = 无限
	seq        int64
}

// NewBlock 创建 Memory Block
func NewBlock(maxEntries int) *Block {
	return &Block{maxEntries: maxEntries}
}

// Inject 增量注入记忆（去重）。返回实际注入的条数
func (b *Block) Inject(items []Entry) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	seen := make(map[string]struct{}, len(b.entries))
	for _, e := range b.entries {
		seen[e.Text] = struct{}{}
	}

	injected := 0
	for _, item := range items {
		if item.Text == "" {
			continue
		}
		if _, dup := seen[item.Text]; dup {
			continue
		}
		seen[item.Text] = struct{}{}
		b.seq++
		item.ID = b.seq
		if item.CreateAt.IsZero() {
			item.CreateAt = time.Now()
		}
		b.entries = append(b.entries, item)
		injected++
	}

	b.trim()
	return injected
}

// trim 超出容量时丢弃最旧的
func (b *Block) trim() {
	if b.maxEntries <= 0 {
		return
	}
	if len(b.entries) > b.maxEntries {
		overflow := len(b.entries) - b.maxEntries
		b.entries = append([]Entry(nil), b.entries[overflow:]...)
	}
}

// List 返回全部条目（副本）
func (b *Block) List() []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]Entry, len(b.entries))
	copy(out, b.entries)
	return out
}

// Get 返回单条
func (b *Block) Get(id int64) (Entry, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, e := range b.entries {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// Len 当前条数
func (b *Block) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.entries)
}

// SetMaxEntries 热重载：调整容量上限（变小立即截断）
func (b *Block) SetMaxEntries(max int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.maxEntries = max
	b.trim()
}

// Clear 清空（仅测试用；前端不可删）
func (b *Block) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = nil
	b.seq = 0
}

// InjectIfAbsent 便捷方法：注入纯文本记忆
func (b *Block) InjectIfAbsent(text string, role string, source string) bool {
	return b.Inject([]Entry{{Role: role, Text: text, Source: source}}) > 0
}
