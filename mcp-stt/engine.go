package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ==================== OpenAI 兼容 STT API（硅基流动 SenseVoice 等） ====================

func transcribeViaAPI(path string) (string, error) {
	baseURL := strings.TrimSuffix(envOr("ALICE_STT_BASE_URL", "https://api.siliconflow.cn/v1"), "/")
	model := envOr("ALICE_STT_MODEL", "iic/SenseVoiceSmall")

	// multipart/form-data: file + model
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(fw, f); err != nil {
		return "", err
	}
	if err := w.WriteField("model", model); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/audio/transcriptions", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+os.Getenv("ALICE_STT_API_KEY"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("STT 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("STT 返回 %d: %s", resp.StatusCode, string(data))
	}

	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	text := strings.TrimSpace(out.Text)
	if text == "" {
		return "", fmt.Errorf("STT 未识别出内容")
	}
	return text, nil
}

// ==================== 本地 whisper.cpp CLI 兜底 ====================

func transcribeViaWhisper(path string) (string, error) {
	bin := envOr("ALICE_WHISPER_BIN", "whisper-cli")
	model := envOr("ALICE_WHISPER_MODEL", "models/ggml-base.bin")

	// whisper.cpp 输出到临时目录：-otxt -of <prefix> 生成 <prefix>.txt
	outDir, err := os.MkdirTemp("", "alice-whisper-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(outDir)
	prefix := filepath.Join(outDir, "out")

	args := []string{"-m", model, "-f", path, "-l", "zh", "-otxt", "-of", prefix, "--no-prints"}
	cmd := exec.Command(bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("本地 whisper 执行失败（%v）。请配置 ALICE_STT_API_KEY 或安装 whisper.cpp: %s", err, truncateStr(stderr.String(), 200))
	}

	data, err := os.ReadFile(prefix + ".txt")
	if err != nil {
		return "", fmt.Errorf("读取 whisper 输出失败: %w", err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("本地 whisper 未识别出内容")
	}
	return text, nil
}

func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
