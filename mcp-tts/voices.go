package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// ==================== Edge TTS 音色列表 ====================

const edgeVoiceListURL = "https://speech.platform.bing.com/consumer/speech/synthesize/readaloud/voices/list?trustedclienttoken="

// VoiceInfo 一个音色条目（返回给调用方）
type VoiceInfo struct {
	ShortName   string `json:"short_name"`
	FriendlyName string `json:"friendly_name"`
	Gender      string `json:"gender"`
	Locale      string `json:"locale"`
	Status      int    `json:"status"` // 2=可用, 3=预览
}

// voiceCache 音色列表缓存（微软列表变化不频繁）
var voiceCache struct {
	sync.Mutex
	data    []VoiceInfo
	fetched time.Time
}

const voiceCacheTTL = 6 * time.Hour

// fallbackVoices 兜底音色（列表拉取失败时返回，保证功能可用）
var fallbackVoices = []VoiceInfo{
	{ShortName: "en-US-EmmaMultilingualNeural", FriendlyName: "Emma (Multilingual)", Gender: "Female", Locale: "en-US", Status: 2},
	{ShortName: "zh-CN-XiaoyiNeural", FriendlyName: "Xiaoyi - 晓伊", Gender: "Female", Locale: "zh-CN", Status: 2},
	{ShortName: "zh-CN-YunxiNeural", FriendlyName: "Yunxi - 云希", Gender: "Male", Locale: "zh-CN", Status: 2},
	{ShortName: "zh-CN-XiaoxiaoNeural", FriendlyName: "Xiaoxiao - 晓晓", Gender: "Female", Locale: "zh-CN", Status: 2},
	{ShortName: "zh-CN-YunjianNeural", FriendlyName: "Yunjian - 云健", Gender: "Male", Locale: "zh-CN", Status: 2},
	{ShortName: "zh-CN-XiaochenNeural", FriendlyName: "Xiaochen - 晓辰", Gender: "Female", Locale: "zh-CN", Status: 2},
	{ShortName: "zh-TW-HsiaoChenNeural", FriendlyName: "HsiaoChen - 曉臻", Gender: "Female", Locale: "zh-TW", Status: 2},
	{ShortName: "zh-HK-HiuMaanNeural", FriendlyName: "HiuMaan - 曉曼", Gender: "Female", Locale: "zh-HK", Status: 2},
}

// listVoices 拉取微软音色列表（带缓存与兜底）
// locale 过滤：空返回全部；"zh" 返回中文相关（zh-CN/zh-TW/zh-HK + 多语言）
func listVoices(locale string) ([]VoiceInfo, error) {
	voiceCache.Lock()
	defer voiceCache.Unlock()

	if len(voiceCache.data) > 0 && time.Since(voiceCache.fetched) < voiceCacheTTL {
		return filterVoices(voiceCache.data, locale), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, edgeVoiceListURL+edgeTrustedToken, nil)
	if err != nil {
		return fallbackVoices, err
	}
	req.Header.Set("User-Agent", edgeUserAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fallbackVoices, fmt.Errorf("拉取音色列表失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fallbackVoices, fmt.Errorf("音色列表返回 %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fallbackVoices, err
	}

	var raw []struct {
		Name         string `json:"Name"`
		ShortName    string `json:"ShortName"`
		Gender       string `json:"Gender"`
		Locale       string `json:"Locale"`
		FriendlyName string `json:"FriendlyName"`
		Status       json.RawMessage `json:"Status"` // 微软返回数字或字符串，兼容两种
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fallbackVoices, fmt.Errorf("解析音色列表失败: %w", err)
	}

	out := make([]VoiceInfo, 0, len(raw))
	for _, v := range raw {
		if v.ShortName == "" {
			continue
		}
		status := parseVoiceStatus(v.Status)
		out = append(out, VoiceInfo{
			ShortName:    v.ShortName,
			FriendlyName: v.FriendlyName,
			Gender:       v.Gender,
			Locale:       v.Locale,
			Status:       status,
		})
	}
	if len(out) == 0 {
		return fallbackVoices, fmt.Errorf("音色列表为空")
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Locale < out[j].Locale })
	voiceCache.data = out
	voiceCache.fetched = time.Now()
	return filterVoices(out, locale), nil
}

// parseVoiceStatus 兼容数字与字符串两种 Status
func parseVoiceStatus(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		// 兼容 "2" / "2.0" 等
		var f float64
		if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
			return int(f)
		}
	}
	return 0
}

// filterVoices 按 locale 过滤；"zh" 匹配中文相关（含多语言音色）
func filterVoices(list []VoiceInfo, locale string) []VoiceInfo {
	if locale == "" {
		return list
	}
	out := make([]VoiceInfo, 0, len(list))
	for _, v := range list {
		if locale == "zh" {
			if strings.HasPrefix(v.Locale, "zh-") || strings.Contains(v.ShortName, "Multilingual") {
				out = append(out, v)
			}
			continue
		}
		if v.Locale == locale {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return list // 过滤无结果时返回全部，避免空列表
	}
	return out
}
