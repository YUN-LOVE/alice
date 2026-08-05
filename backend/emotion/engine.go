package emotion

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"alice/config"
)

// Engine 情绪引擎：高维向量 + 事件驱动 + 时间演化 + 阈值触发
type Engine struct {
	cfg      *config.EmotionConfig
	rdb      *redis.Client
	mu       sync.Mutex
	vector   map[string]float64
	lastTick time.Time
	// 主动推送状态
	lastPush time.Time
	// 事件匹配缓存
	eventKeywords map[string][]string
}

// New 创建情绪引擎，尝试从 Redis 恢复持久化状态
func New(cfg *config.EmotionConfig, redisAddr, redisPassword string, redisDB int) *Engine {
	e := &Engine{
		cfg:           cfg,
		vector:        make(map[string]float64),
		lastTick:      time.Now(),
		lastPush:      time.Now(),
		eventKeywords: make(map[string][]string),
	}
	if cfg.Persistence.Enabled {
		e.rdb = redis.NewClient(&redis.Options{
			Addr:     redisAddr,
			Password: redisPassword,
			DB:       redisDB,
		})
	}
	for _, d := range cfg.Dimensions {
		e.vector[d] = cfg.Initial[d]
	}
	for name, ev := range cfg.EventMap {
		if len(ev.Keywords) > 0 {
			e.eventKeywords[name] = ev.Keywords
		}
	}

	if cfg.Persistence.Enabled {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := e.load(ctx); err != nil {
			log.Printf("[emotion] 状态恢复失败（使用初始值）: %v", err)
		}
	}
	return e
}

// DetectEvent 根据用户文本识别事件名（关键词匹配）
func (e *Engine) DetectEvent(text string) string {
	lower := strings.ToLower(text)
	best := ""
	for name, keywords := range e.eventKeywords {
		for _, kw := range keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				if best == "" || len(name) > len(best) {
					best = name
				}
				break
			}
		}
	}
	if best == "" {
		return "default"
	}
	return best
}

// ProcessEvent 事件驱动：更新情绪向量（不越界）
func (e *Engine) ProcessEvent(eventName string) map[string]float64 {
	event, ok := e.cfg.EventMap[eventName]
	if !ok {
		event = e.cfg.EventMap["default"]
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for dim, delta := range event.Changes {
		if _, exists := e.vector[dim]; !exists {
			continue
		}
		e.vector[dim] = clamp(e.vector[dim]+delta, 0, e.cfg.MaxValue)
	}
	return e.cloneLocked()
}

// Tick 时间演化：情绪自然衰减趋近 baseline + 关系矩阵漂移/拮抗
func (e *Engine) Tick() {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	dt := now.Sub(e.lastTick).Seconds()
	e.lastTick = now
	if dt <= 0 {
		return
	}

	// 指数衰减逼近 baseline: v' = base + (v - base) * exp(-decayRate * dt)
	factor := math.Exp(-e.cfg.DecayRate * dt)
	for dim, v := range e.vector {
		base := e.cfg.Baseline[dim]
		e.vector[dim] = clamp(base+(v-base)*factor, 0, e.cfg.MaxValue)
	}

	// 关系矩阵：目标维度受来源维度影响（漂移/拮抗）
	for target, sources := range e.cfg.Relations {
		tv, ok := e.vector[target]
		if !ok {
			continue
		}
		for src, coef := range sources {
			sv, ok := e.vector[src]
			if !ok {
				continue
			}
			tv += coef * sv * dt
		}
		e.vector[target] = clamp(tv, 0, e.cfg.MaxValue)
	}
}

// Summary 返回情绪描述文本 + 命中的模板（用于注入上下文）
func (e *Engine) Summary() (description, styleTemplate string, top string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 找最高维度
	top = ""
	maxV := -1.0
	for d, v := range e.vector {
		if v > maxV {
			maxV = v
			top = d
		}
	}

	// 按模板顺序匹配
	for _, tpl := range e.cfg.Templates {
		if matchConditions(e.vector, tpl.Conditions) {
			return tpl.Text, tpl.Name, top
		}
	}
	return "你情绪平稳，按自己的人格自然回应。", "default", top
}

// State 返回当前情绪向量快照
func (e *Engine) State() map[string]float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cloneLocked()
}

// Highest 返回当前最高的情绪维度及值
func (e *Engine) Highest() (string, float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	top, v := "", -1.0
	for d, val := range e.vector {
		if val > v {
			v, top = val, d
		}
	}
	return top, v
}

// ShouldProactive 判断是否应触发主动推送（超阈值 + 冷却期已过）
// 命中后内部重置冷却时间
func (e *Engine) ShouldProactive() (bool, string) {
	if !e.cfg.Proactive.Enabled {
		return false, ""
	}
	_, v := e.Highest()
	if v < e.cfg.Proactive.Threshold {
		return false, ""
	}
	cooldown := time.Duration(e.cfg.Proactive.CooldownSec) * time.Second
	e.mu.Lock()
	defer e.mu.Unlock()
	if time.Since(e.lastPush) < cooldown {
		return false, ""
	}
	e.lastPush = time.Now()
	return true, e.proactiveTextLocked()
}

// proactiveTextLocked 根据当前最高情绪取主动话术
func (e *Engine) proactiveTextLocked() string {
	top, v := "", -1.0
	for d, val := range e.vector {
		if val > v {
			v, top = val, d
		}
	}
	_ = v
	for _, tpl := range e.cfg.Templates {
		if matchConditions(e.vector, tpl.Conditions) {
			if tpl.Proactive != "" {
				return tpl.Proactive
			}
		}
	}
	_ = top
	return "在吗？突然想跟你说句话。"
}

// ==================== 持久化 ====================

const stateKey = "alice:emotion:state"

// Save 持久化情绪向量到 Redis（带 TTL）
func (e *Engine) Save(ctx context.Context) error {
	if !e.cfg.Persistence.Enabled || e.rdb == nil {
		return nil
	}
	data, err := json.Marshal(e.State())
	if err != nil {
		return err
	}
	return e.rdb.Set(ctx, stateKey, data, time.Duration(e.cfg.Persistence.TTLSec)*time.Second).Err()
}

func (e *Engine) load(ctx context.Context) error {
	data, err := e.rdb.Get(ctx, stateKey).Bytes()
	if err != nil {
		return err
	}
	var saved map[string]float64
	if err := json.Unmarshal(data, &saved); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for d, v := range saved {
		if _, exists := e.vector[d]; exists {
			e.vector[d] = clamp(v, 0, e.cfg.MaxValue)
		}
	}
	log.Printf("[emotion] 已恢复持久化情绪状态: %v", e.vector)
	return nil
}

// ==================== 情绪记忆（显著事件记录） ====================

const eventListKey = "alice:emotion:events"

// EmotionEventRecord 一条情绪事件记录
type EmotionEventRecord struct {
	Time    int64            `json:"time"`
	Event   string           `json:"event"`
	Delta   map[string]float64 `json:"delta"`
	State   map[string]float64 `json:"state"`
}

// significantDelta 判断事件是否显著（任一维度变化量超过阈值）
func significantDelta(delta map[string]float64) bool {
	for _, v := range delta {
		if v < -0.1 || v > 0.1 {
			return true
		}
	}
	return false
}

// RecordEvent 记录一次情绪事件（仅显著事件，保留最近 500 条）
func (e *Engine) RecordEvent(eventName string) {
	if e.rdb == nil {
		return
	}
	event, ok := e.cfg.EventMap[eventName]
	if !ok {
		event = e.cfg.EventMap["default"]
	}
	if !significantDelta(event.Changes) {
		return
	}

	e.mu.Lock()
	state := e.cloneLocked()
	e.mu.Unlock()

	rec := EmotionEventRecord{
		Time:  time.Now().Unix(),
		Event: eventName,
		Delta: event.Changes,
		State: state,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pipe := e.rdb.TxPipeline()
	pipe.LPush(ctx, eventListKey, data)
	pipe.LTrim(ctx, eventListKey, 0, 499)
	_, _ = pipe.Exec(ctx)
}

// Events 查询最近的情绪事件（最新在前）
func (e *Engine) Events(ctx context.Context, limit int) ([]EmotionEventRecord, error) {
	if e.rdb == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	data, err := e.rdb.LRange(ctx, eventListKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}
	out := make([]EmotionEventRecord, 0, len(data))
	for _, d := range data {
		var rec EmotionEventRecord
		if err := json.Unmarshal([]byte(d), &rec); err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

// ==================== 工具 ====================

func (e *Engine) cloneLocked() map[string]float64 {
	out := make(map[string]float64, len(e.vector))
	for k, v := range e.vector {
		out[k] = v
	}
	return out
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func matchConditions(vec map[string]float64, cond map[string][2]float64) bool {
	for dim, r := range cond {
		v, ok := vec[dim]
		if !ok {
			return false
		}
		if v < r[0] || v > r[1] {
			return false
		}
	}
	return true
}
