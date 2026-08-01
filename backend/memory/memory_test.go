package memory

import (
	"context"
	"fmt"
	"math"
	"os"
	"testing"
)

// ==================== Embedding ====================

func TestNormalizeAndCosine(t *testing.T) {
	v1 := Normalize([]float64{3, 4})
	if math.Abs(v1[0]-0.6) > 1e-9 || math.Abs(v1[1]-0.8) > 1e-9 {
		t.Fatalf("归一化错误: %v", v1)
	}
	if Cosine(v1, v1) < 0.999999 {
		t.Fatalf("相同向量余弦应≈1: %v", Cosine(v1, v1))
	}
	orth := Normalize([]float64{-4, 3})
	if Cosine(v1, orth) > 1e-9 {
		t.Fatalf("正交向量余弦应≈0: %v", Cosine(v1, orth))
	}
}

func TestHashEmbedderDeterministic(t *testing.T) {
	e := &HashEmbedder{}
	ctx := context.Background()
	a, _ := e.Embed(ctx, "今天好累，工作压力好大")
	b, _ := e.Embed(ctx, "今天好累，工作压力好大")
	c, _ := e.Embed(ctx, "今天天气很好")
	if Cosine(a, b) < 0.999999 {
		t.Fatalf("相同文本向量应一致")
	}
	if Cosine(a, c) >= Cosine(a, b) {
		t.Fatalf("相关文本应比无关文本相似度更高")
	}
}

// ==================== Memory Block ====================

func TestBlockInjectDedupe(t *testing.T) {
	b := NewBlock(100)
	n := b.Inject([]Entry{{Role: "user", Text: "今天好累"}, {Role: "user", Text: "今天好累"}})
	if n != 1 {
		t.Fatalf("重复注入应只计入 1 条，实际 %d", n)
	}
	if b.Len() != 1 {
		t.Fatalf("Block 应有 1 条，实际 %d", b.Len())
	}
}

func TestBlockCapacity(t *testing.T) {
	b := NewBlock(3)
	for i := 0; i < 5; i++ {
		b.InjectIfAbsent(fmt.Sprintf("msg-%d", i), "user", "test")
	}
	if b.Len() != 3 {
		t.Fatalf("超出容量应截断为 3 条，实际 %d", b.Len())
	}
	entries := b.List()
	if entries[0].Text != "msg-2" {
		t.Fatalf("应丢弃最旧条目，最旧应为 msg-2，实际 %s", entries[0].Text)
	}
}

func TestBlockZeroMeansUnlimited(t *testing.T) {
	b := NewBlock(0)
	for i := 0; i < 500; i++ {
		b.InjectIfAbsent(fmt.Sprintf("msg-%d", i), "user", "test")
	}
	if b.Len() != 500 {
		t.Fatalf("0 应表示无限容量，实际 %d", b.Len())
	}
}

// ==================== RAG ====================

// 用真实 Redis 测试检索链路（未提供 REDIS_ADDR 时跳过）
func TestRAGRetrieve(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("未设置 REDIS_ADDR，跳过 RAG 集成测试")
	}

	ctx := context.Background()
	rag := NewRAG(addr, "", 15, &HashEmbedder{}, 5, 0)
	defer rag.Close()

	rag.Store(ctx, "user", "我最近在做一个叫 Alice 的项目", nil)
	rag.Store(ctx, "user", "今天中午吃了牛肉面", nil)

	results, err := rag.Retrieve(ctx, "Alice 项目进展如何")
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("应检索到相关记忆")
	}
	if results[0].Mem.Text != "我最近在做一个叫 Alice 的项目" {
		t.Fatalf("最相关记忆不匹配: %s", results[0].Mem.Text)
	}
}
