package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"
)

// ==================== OpenAI 兼容 TTS API（硅基流动 CosyVoice 等） ====================

func speakViaAPI(text, voice string) (*speakResult, error) {
	baseURL := strings.TrimSuffix(envOr("ALICE_TTS_BASE_URL", "https://api.siliconflow.cn/v1"), "/")
	model := envOr("ALICE_TTS_MODEL", "FunAudioLLM/CosyVoice2-0.5B")
	if voice == "" {
		voice = envOr("ALICE_TTS_VOICE", "female-calm")
	}

	payload := map[string]any{
		"model": model,
		"input": text,
		"voice": voice,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/audio/speech", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("ALICE_TTS_API_KEY"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TTS 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("TTS 返回 %d: %s", resp.StatusCode, string(data))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("TTS 返回空音频")
	}

	format := "mp3"
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		if ext, _ := mime.ExtensionsByType(strings.TrimSpace(strings.Split(ct, ";")[0])); len(ext) > 0 {
			format = strings.TrimPrefix(ext[0], ".")
		}
	}

	return &speakResult{
		Audio:       base64.StdEncoding.EncodeToString(data),
		Format:      format,
		DurationSec: estimateDuration(len(data), format),
	}, nil
}

// estimateDuration 粗略估算音频时长（按码率）：mp3 ~16KB/s，wav 48kHz 16bit 单声道 ~96KB/s
func estimateDuration(byteLen int, format string) float64 {
	rate := 16000.0
	switch format {
	case "wav":
		rate = 96000.0
	case "ogg", "opus":
		rate = 16000.0
	}
	return float64(byteLen) / rate
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
