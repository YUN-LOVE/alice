package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	memKeyPrefix   = "alice:mem:"     // hash: 记忆详情
	seqKey         = "alice:mem:seq"  // 计数器
	dateKeyPrefix  = "alice:mem:date:" // SET: 某日期下的记忆 ID（每日归档）
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

// Store 存储一条记忆（自动生成向量 + 按日期归档）
// meta 中 "date"（YYYY-MM-DD）决定归档日期；缺省用当天
func (r *RAG) Store(ctx context.Context, role, text string, meta map[string]any) (int64, error) {
	vec, err := r.embedder.Embed(ctx, text)
	if err != nil {
		return 0, fmt.Errorf("生成向量失败: %w", err)
	}

	id, err := r.rdb.Incr(ctx, seqKey).Result()
	if err != nil {
		return 0, err
	}

	date := time.Now().Format("2006-01-02")
	if meta != nil {
		if d, ok := meta["date"].(string); ok && d != "" {
			date = d
		}
	}

	m := Mem{
		ID:       id,
		Role:     role,
		Text:     text,
		Vector:   vec,
		CreateAt: time.Now(),
		Meta:     meta,
	}
	if err := r.save(ctx, m); err != nil {
		return 0, err
	}
	// 加入日期归档集合
	if err := r.rdb.SAdd(ctx, dateKeyPrefix+date, id).Err(); err != nil {
		return 0, err
	}
	return id, nil
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

// RetrieveByDate 返回某日期归档的对话（按时间升序，用于前端加载历史聊天记录）
func (r *RAG) RetrieveByDate(ctx context.Context, date string) ([]Mem, error) {
	ids, err := r.rdb.SMembers(ctx, dateKeyPrefix+date).Result()
	if err != nil {
		return nil, err
	}
	out := make([]Mem, 0, len(ids))
	for _, idStr := range ids {
		data, err := r.rdb.Get(ctx, memKeyPrefix+idStr).Bytes()
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
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreateAt.Before(out[j].CreateAt)
	})
	return out, nil
}

// CountByDate 返回某日期归档的记忆条数
func (r *RAG) CountByDate(ctx context.Context, date string) (int64, error) {
	return r.rdb.SCard(ctx, dateKeyPrefix+date).Result()
}

// Dates 返回已有归档的日期列表（降序）
func (r *RAG) Dates(ctx context.Context) ([]string, error) {
	keys, err := r.rdb.Keys(ctx, dateKeyPrefix+"*").Result()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, strings.TrimPrefix(k, dateKeyPrefix))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out, nil
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

// Reconfigure 热重载：替换 Embedding / 检索参数（Redis 连接与数据保留）
func (r *RAG) Reconfigure(embedder Embedder, topK int, minScore float64) {
	r.embedder = embedder
	r.topK = topK
	r.minScore = minScore
}

// Close 关闭连接
func (r *RAG) Close() error { return r.rdb.Close() }
