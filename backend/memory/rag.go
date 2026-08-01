package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	memKeyPrefix = "alice:mem:"   // hash: 记忆详情
	seqKey       = "alice:mem:seq" // 计数器
)

// Mem 一条长期记忆
type Mem struct {
	ID       int64          `json:"id"`
	Role     string         `json:"role"` // user / assistant
	Text     string         `json:"text"` // 对话原文
	Vector   []float64      `json:"vector"`
	CreateAt time.Time      `json:"create_at"`
	Meta     map[string]any `json:"meta,omitempty"` // 情绪快照等
}

// SearchResult 检索结果
type SearchResult struct {
	Mem      Mem     `json:"mem"`
	Score    float64 `json:"score"`
}

// RAG 长期记忆：
// - 全量存储每轮对话原文 + 元数据
// - 云端 Embedding 生成向量，本地 Redis 存储
// - 检索：Go 侧余弦相似度（数据量万级内毫秒级；接口已抽象，可换 RediSearch）
type RAG struct {
	rdb      *redis.Client
	embedder Embedder
	topK     int
	minScore float64
}

// NewRAG 创建 RAG
func NewRAG(addr, password string, db int, embedder Embedder, topK int, minScore float64) *RAG {
	return &RAG{
		rdb: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
		}),
		embedder: embedder,
		topK:     topK,
		minScore: minScore,
	}
}

// Ping 检查 Redis 连通性
func (r *RAG) Ping(ctx context.Context) error { return r.rdb.Ping(ctx).Err() }

// Store 存储一条记忆（自动生成向量）
func (r *RAG) Store(ctx context.Context, role, text string, meta map[string]any) (int64, error) {
	vec, err := r.embedder.Embed(ctx, text)
	if err != nil {
		return 0, fmt.Errorf("生成向量失败: %w", err)
	}

	id, err := r.rdb.Incr(ctx, seqKey).Result()
	if err != nil {
		return 0, err
	}

	m := Mem{
		ID:       id,
		Role:     role,
		Text:     text,
		Vector:   vec,
		CreateAt: time.Now(),
		Meta:     meta,
	}
	return id, r.save(ctx, m)
}

func (r *RAG) save(ctx context.Context, m Mem) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return r.rdb.Set(ctx, memKeyPrefix+fmt.Sprint(m.ID), data, 0).Err()
}

// Retrieve 检索与 query 最相关的 topK 条记忆
func (r *RAG) Retrieve(ctx context.Context, query string) ([]SearchResult, error) {
	vec, err := r.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	keys, err := r.rdb.Keys(ctx, memKeyPrefix+"*").Result()
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(keys))
	for _, key := range keys {
		data, err := r.rdb.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		var m Mem
		if err := json.Unmarshal(data, &m); err != nil || m.Vector == nil {
			continue
		}
		score := Cosine(vec, m.Vector)
		if score < r.minScore {
			continue
		}
		m.Vector = nil // 不返回向量
		results = append(results, SearchResult{Mem: m, Score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > r.topK {
		results = results[:r.topK]
	}
	return results, nil
}

// All 导出全部记忆
func (r *RAG) All(ctx context.Context) ([]Mem, error) {
	keys, err := r.rdb.Keys(ctx, memKeyPrefix+"*").Result()
	if err != nil {
		return nil, err
	}
	out := make([]Mem, 0, len(keys))
	for _, key := range keys {
		data, err := r.rdb.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		var m Mem
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		m.Vector = nil
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Count 记忆总数
func (r *RAG) Count(ctx context.Context) (int64, error) {
	keys, err := r.rdb.Keys(ctx, memKeyPrefix+"*").Result()
	if err != nil {
		return 0, err
	}
	return int64(len(keys)), nil
}

// EmbedderName 返回当前 Embedding 名称
func (r *RAG) EmbedderName() string { return r.embedder.Name() }

// Close 关闭连接
func (r *RAG) Close() error { return r.rdb.Close() }
