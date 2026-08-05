package emotion

import (
	"testing"
	"time"

	"alice/config"
)

func testConfig() *config.EmotionConfig {
	return &config.EmotionConfig{
		Dimensions: []string{"开心", "失落", "温柔", "焦虑", "好奇"},
		Initial: map[string]float64{
			"开心": 0.4, "失落": 0.1, "温柔": 0.3, "焦虑": 0.1, "好奇": 0.5,
		},
		Baseline: map[string]float64{
			"开心": 0.4, "失落": 0.1, "温柔": 0.3, "焦虑": 0.1, "好奇": 0.5,
		},
		DecayRate: 0.01,
		MaxValue:  1.0,
		Proactive: struct {
			Enabled         bool    `yaml:"enabled"`
			Threshold       float64 `yaml:"threshold"`
			CooldownSec     int     `yaml:"cooldown_seconds"`
			TickSec         int     `yaml:"tick_seconds"`
			SilentAfterMin  int     `yaml:"silent_after_minutes"`
			SkipIfActiveMin int     `yaml:"skip_if_active_minutes"`
			Hours           []int   `yaml:"hours"`
		}{Enabled: true, Threshold: 0.7, CooldownSec: 600, TickSec: 1},
		Persistence: struct {
			Enabled bool `yaml:"enabled"`
			TTLSec  int  `yaml:"ttl_seconds"`
		}{Enabled: false},
		EventMap: map[string]config.EmotionEvent{
			"user_share_bad_news": {Keywords: []string{"好累"}, Changes: map[string]float64{"失落": 0.15, "温柔": 0.1}},
			"user_greeting":       {Keywords: []string{"你好"}, Changes: map[string]float64{"开心": 0.05}},
			"default":             {Changes: map[string]float64{"温柔": 0.01}},
		},
		Templates: []config.EmotionTemplate{
			{Name: "down", Conditions: map[string][2]float64{"失落": {0.4, 1.0}}, Text: "你此刻有些低落", Proactive: "感觉有点闷闷的"},
			{Name: "happy", Conditions: map[string][2]float64{"开心": {0.6, 1.0}}, Text: "你此刻心情很好", Proactive: "突然想你了"},
			{Name: "calm", Conditions: map[string][2]float64{"开心": {0.2, 0.6}}, Text: "你此刻心情平静", Proactive: "最近怎么样"},
		},
	}
}

func newTestEngine() *Engine {
	return New(testConfig(), "", "", 0)
}

func TestDetectEvent(t *testing.T) {
	e := newTestEngine()
	cases := map[string]string{
		"今天好累啊": "user_share_bad_news",
		"你好呀":     "user_greeting",
		"随便聊聊":    "default",
	}
	for input, want := range cases {
		if got := e.DetectEvent(input); got != want {
			t.Errorf("DetectEvent(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestProcessEventClamp(t *testing.T) {
	e := newTestEngine()
	// 连发多个坏消息事件，失落不应超过上限 1.0
	for i := 0; i < 20; i++ {
		e.ProcessEvent("user_share_bad_news")
	}
	state := e.State()
	if state["失落"] > 1.0+1e-9 {
		t.Fatalf("情绪越界: 失落=%v", state["失落"])
	}
	if state["失落"] <= 0.9 {
		t.Fatalf("情绪未累积: 失落=%v", state["失落"])
	}
}

func TestTickDecay(t *testing.T) {
	e := newTestEngine()
	// 先推高开心，再模拟长时间衰减，应回落趋近 baseline
	e.ProcessEvent("user_greeting")
	e.ProcessEvent("user_greeting")
	state := e.State()
	happyBefore := state["开心"]
	if happyBefore <= 0.4 {
		t.Fatalf("开心未升高: %v", happyBefore)
	}
	e.lastTick = e.lastTick.Add(-time.Hour) // 假装 1 小时前
	e.Tick()
	happyAfter := e.State()["开心"]
	if happyAfter >= happyBefore {
		t.Fatalf("衰减无效: before=%v after=%v", happyBefore, happyAfter)
	}
	if happyAfter < 0.35 || happyAfter > 0.41 {
		t.Fatalf("衰减应趋近 baseline 0.4，实际 %v", happyAfter)
	}
}

func TestSummaryTemplateMatch(t *testing.T) {
	e := newTestEngine()
	// 多次倾诉后失落到 0.4 以上，匹配 down 模板
	for i := 0; i < 3; i++ {
		e.ProcessEvent("user_share_bad_news")
	}
	desc, tpl, _ := e.Summary()
	if tpl != "down" {
		t.Fatalf("期望 down 模板，实际 %s（描述: %s，状态: %v）", tpl, desc, e.State())
	}
}

func TestShouldProactive(t *testing.T) {
	e := newTestEngine()
	// 初始无维度超阈值
	if ok, _ := e.ShouldProactive(); ok {
		t.Fatal("初始状态不应触发主动推送")
	}
	// 推高失落到超阈值
	for i := 0; i < 10; i++ {
		e.ProcessEvent("user_share_bad_news")
	}
	// 重置冷却时间以便测试（冷却 600s，回拨 601s）
	e.lastPush = e.lastPush.Add(-601 * time.Second)
	ok, text := e.ShouldProactive()
	if !ok {
		t.Fatalf("超阈值应触发主动推送")
	}
	if text == "" {
		t.Fatal("主动推送话术不能为空")
	}
	// 冷却期内不应再次触发
	if ok2, _ := e.ShouldProactive(); ok2 {
		t.Fatal("冷却期内不应重复触发")
	}
}

// TestRelationsMatrix 关系矩阵：焦虑高抑制开心
func TestRelationsMatrix(t *testing.T) {
	cfg := testConfig()
	cfg.Relations = map[string]map[string]float64{
		"开心": {"焦虑": -0.5},
	}
	e := New(cfg, "", "", 0)

	// 推高焦虑
	for i := 0; i < 10; i++ {
		e.ProcessEvent("user_argue")
	}
	e.lastTick = e.lastTick.Add(-time.Second)
	happyBefore := e.State()["开心"]

	// tick 多次，开心应被焦虑抑制下降
	for i := 0; i < 10; i++ {
		e.lastTick = e.lastTick.Add(-time.Second)
		e.Tick()
	}
	happyAfter := e.State()["开心"]
	if happyAfter >= happyBefore-1e-9 {
		t.Fatalf("关系矩阵未生效：开心 %v → %v（焦虑 %v）", happyBefore, happyAfter, e.State()["焦虑"])
	}
}

// TestSignificantDelta 显著事件判断
func TestSignificantDelta(t *testing.T) {
	if !significantDelta(map[string]float64{"开心": 0.2}) {
		t.Fatal("0.2 变化应为显著")
	}
	if significantDelta(map[string]float64{"温柔": 0.05}) {
		t.Fatal("0.05 变化不应为显著")
	}
	if significantDelta(map[string]float64{}) {
		t.Fatal("空变化不应为显著")
	}
}
