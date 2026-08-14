package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// ==================== Edge TTS（免费，无需 key） ====================

const (
	edgeTrustedToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	edgeWSSBase      = "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1"
	edgeGecVersion   = "1-130.0.2849.68"
	edgeOutputFormat = "audio-24khz-48kbitrate-mono-mp3"
)

// edgeVoices 默认音色候选（微软会不定期下架音色，逐个尝试）
var edgeVoices = []string{
	"zh-CN-XiaoxiaoNeural",
	"zh-CN-XiaoyiNeural",
	"zh-CN-YunxiNeural",
}

// edgeSSML 构造 SSML 请求体（文本需 XML 转义）
func edgeSSML(text, voice string) string {
	esc := strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;",
	)
	return fmt.Sprintf(
		"<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='zh-CN'>"+
			"<voice name='%s'><prosody pitch='+0Hz' rate='+0%%' volume='+0%%'>%s</prosody></voice></speak>",
		voice, esc.Replace(text),
	)
}

// edgeSpeechConfig speech.config 上下文消息
func edgeSpeechConfig() string {
	data, _ := json.Marshal(map[string]any{
		"context": map[string]any{
			"synthesis": map[string]any{
				"audio": map[string]any{
					"metadataoptions": map[string]any{
						"sentenceBoundaryEnabled": "false",
						"wordBoundaryEnabled":     "true",
					},
					"outputFormat": edgeOutputFormat,
				},
			},
		},
	})
	return string(data)
}

// edgeGec 计算 Sec-MS-GEC：UTC 当前小时 + TrustedClientToken 的 SHA256
func edgeGec() string {
	now := time.Now().UTC().Format("2006-01-02T15") // 按小时变化（对齐 edge-tts）
	sum := sha256.Sum256([]byte(now + edgeTrustedToken))
	return hex.EncodeToString(sum[:])
}

func edgeWSURL() string {
	u := edgeWSSBase + "?" + url.Values{
		"TrustedClientToken": {edgeTrustedToken},
		"Sec-MS-GEC":         {edgeGec()},
		"Sec-MS-GEC-Version": {edgeGecVersion},
		"ConnectionId":       {uuid4()},
	}.Encode()
	return u
}

func uuid4() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// edgeSynthesize 通过 Edge TTS WebSocket 合成语音，返回 MP3 字节
func edgeSynthesize(text, voice string) ([]byte, error) {
	headers := http.Header{
		"Pragma":          {"no-cache"},
		"Cache-Control":   {"no-cache"},
		"Origin":          {"chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold"},
		"Accept-Encoding": {"gzip, deflate, br"},
		"Accept-Language": {"en-US,en;q=0.9"},
		"User-Agent":      {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0"},
	}

	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, resp, err := dialer.Dial(edgeWSURL(), headers)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("Edge TTS 连接失败: %v (HTTP %d)", err, resp.StatusCode)
		}
		return nil, fmt.Errorf("Edge TTS 连接失败: %v", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return nil, err
	}

	// 1. speech.config
	if err := conn.WriteMessage(websocket.TextMessage, []byte(edgeSpeechConfig())); err != nil {
		return nil, fmt.Errorf("发送 speech.config 失败: %w", err)
	}
	// 2. SSML
	if err := conn.WriteMessage(websocket.TextMessage, []byte(edgeSSML(text, voice))); err != nil {
		return nil, fmt.Errorf("发送 SSML 失败: %w", err)
	}

	var audio []byte
	done := false
	for !done {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if len(audio) > 0 {
				break // 音频已收完，连接关闭可接受
			}
			return nil, fmt.Errorf("Edge TTS 读取失败: %w", err)
		}
		if len(data) == 0 {
			continue
		}
		switch data[0] {
		case 0x4B: // 'K'：元数据（含 turn.end 标记）
			if len(data) >= 3 {
				n := int(binary.BigEndian.Uint16(data[1:3]))
				if n > 0 && len(data) >= 3+n {
					var meta struct {
						Type string `json:"type"`
					}
					if err := json.Unmarshal(data[3:3+n], &meta); err == nil && meta.Type == "turn.end" {
						done = true
					}
				}
			}
		case 0x7B: // '{'：音频数据（前 2 字节为大端长度）
			if len(data) >= 3 {
				n := int(binary.BigEndian.Uint16(data[1:3]))
				if len(data) >= 3+n {
					audio = append(audio, data[3:3+n]...)
				}
			}
		}
	}

	if len(audio) == 0 {
		return nil, fmt.Errorf("Edge TTS 未返回音频")
	}
	return audio, nil
}

// speakViaEdge 尝试默认音色列表，全部失败则报错
func speakViaEdge(text, voice string) (*speakResult, error) {
	candidates := edgeVoices
	if voice != "" {
		candidates = []string{voice}
	}
	var lastErr error
	for _, v := range candidates {
		data, err := edgeSynthesize(text, v)
		if err == nil {
			return &speakResult{
				Audio:       base64.StdEncoding.EncodeToString(data),
				Format:      "mp3",
				DurationSec: estimateDuration(len(data), "mp3"),
			}, nil
		}
		lastErr = err
	}
	if len(candidates) > 1 {
		return nil, fmt.Errorf("Edge TTS 全部音色失败，最后错误: %v", lastErr)
	}
	return nil, lastErr
}
