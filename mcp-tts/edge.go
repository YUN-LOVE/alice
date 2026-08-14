package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// ==================== Edge TTS（免费，无需 key） ====================
// 协议对齐 edge-tts 7.2.8（2026-03）：
// - Sec-MS-GEC：Windows FILETIME（1601 epoch）按 5 分钟取整，SHA256 大写 hex
// - 必须携带 Cookie: muid=<32位大写HEX>
// - speech.config / SSML 均为带 Path 头的文本消息
// - 响应：TEXT 消息携带 Path 参数（turn.end 等）；BINARY 消息前 2 字节为 header 长度

const (
	edgeTrustedToken  = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	edgeWSSBase       = "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1"
	edgeGecVersion    = "1-143.0.3650.75"
	edgeOutputFormat  = "audio-24khz-48kbitrate-mono-mp3"
	edgeWinEpoch      = 11644473600 // 1601-01-01 到 1970-01-01 的秒数
	edgeGecRoundSec   = 300         // GEC 取整粒度：5 分钟
	edgeUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
	edgeDateLayout    = "Mon Jan 02 2006 15:04:05 GMT+0000 (Coordinated Universal Time)"
)

// edgeVoices 音色候选（未指定 ALICE_TTS_EDGE_VOICE 时按序尝试）：
// 官方默认 en-US-EmmaMultilingualNeural（多语言，可读中文），中文音色次选
var edgeVoices = []string{
	"en-US-EmmaMultilingualNeural",
	"zh-CN-XiaoyiNeural",
	"zh-CN-YunxiNeural",
	"zh-CN-XiaoxiaoNeural",
}

// edgeTTSParams Edge TTS 合成参数（环境变量默认值 + 工具参数覆盖）
type edgeTTSParams struct {
	Voice  string // 音色
	Rate   string // 语速，如 "+10%" / "-20%"
	Pitch  string // 音高，如 "+5Hz" / "-3Hz"
	Volume string // 音量，如 "+10%" / "-50%"
}

// edgeParams 读取环境变量默认值：
//   ALICE_TTS_EDGE_VOICE   音色（留空用候选列表）
//   ALICE_TTS_EDGE_RATE    语速（默认 +0%）
//   ALICE_TTS_EDGE_PITCH   音高（默认 +0Hz）
//   ALICE_TTS_EDGE_VOLUME  音量（默认 +0%）
func edgeParams(overrides edgeTTSParams) edgeTTSParams {
	p := edgeTTSParams{
		Voice:  envOr("ALICE_TTS_EDGE_VOICE", ""),
		Rate:   envOr("ALICE_TTS_EDGE_RATE", "+0%"),
		Pitch:  envOr("ALICE_TTS_EDGE_PITCH", "+0Hz"),
		Volume: envOr("ALICE_TTS_EDGE_VOLUME", "+0%"),
	}
	if overrides.Voice != "" {
		p.Voice = overrides.Voice
	}
	if overrides.Rate != "" {
		p.Rate = overrides.Rate
	}
	if overrides.Pitch != "" {
		p.Pitch = overrides.Pitch
	}
	if overrides.Volume != "" {
		p.Volume = overrides.Volume
	}
	return p
}

// edgeSSML 构造 SSML 请求体（xml:lang 跟随官方固定 en-US，中文音色同样适用）
func edgeSSML(text, voice, rate, pitch, volume string) string {
	esc := strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;",
	)
	return fmt.Sprintf(
		"<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='en-US'>"+
			"<voice name='%s'><prosody pitch='%s' rate='%s' volume='%s'>%s</prosody></voice></speak>",
		voice, pitch, rate, volume, esc.Replace(text),
	)
}

// edgeGec 计算 Sec-MS-GEC：当前时间的 Windows FILETIME 值按 5 分钟向下取整，
// 拼接 TrustedClientToken 后 SHA256（大写 hex）
func edgeGec() string {
	ticks := float64(time.Now().Unix()) + edgeWinEpoch
	ticks -= math.Mod(ticks, edgeGecRoundSec)
	ticks *= 1e7 // 转换为 100 纳秒间隔
	sum := sha256.Sum256([]byte(fmt.Sprintf("%.0f%s", ticks, edgeTrustedToken)))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// edgeMuid 生成随机 MUID（32 位大写 hex，Cookie 用）
func edgeMuid() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

func edgeDateString() string {
	return time.Now().UTC().Format(edgeDateLayout)
}

// edgeSpeechConfig speech.config 消息（带 Path 头的文本消息）
func edgeSpeechConfig() string {
	return "X-Timestamp:" + edgeDateString() + "\r\n" +
		"Content-Type:application/json; charset=utf-8\r\n" +
		"Path:speech.config\r\n\r\n" +
		`{"context":{"synthesis":{"audio":{"metadataoptions":{` +
		`"sentenceBoundaryEnabled":"true","wordBoundaryEnabled":"false"},` +
		`"outputFormat":"` + edgeOutputFormat + `"}}}}` + "\r\n"
}

// edgeSSMLMessage SSML 请求消息（X-RequestId + Path:ssml 头）
func edgeSSMLMessage(text, voice, rate, pitch, volume string) string {
	return "X-RequestId:" + uuidHex() + "\r\n" +
		"Content-Type:application/ssml+xml\r\n" +
		"X-Timestamp:" + edgeDateString() + "Z\r\n" + // 官方 bug：多一个 Z，照抄
		"Path:ssml\r\n\r\n" +
		edgeSSML(text, voice, rate, pitch, volume)
}

// edgeCleanText 移除服务端不支持的控制字符（对齐官方 remove_incompatible_characters）
func edgeCleanText(text string) string {
	runes := []rune(text)
	for i, r := range runes {
		if (r >= 0 && r <= 8) || (r >= 11 && r <= 12) || (r >= 14 && r <= 31) {
			runes[i] = ' '
		}
	}
	return string(runes)
}

func uuidHex() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b)
}

// edgeWSURL 构造带鉴权参数的 WSS 地址
func edgeWSURL() string {
	return edgeWSSBase + "?" + url.Values{
		"TrustedClientToken": {edgeTrustedToken},
		"ConnectionId":       {uuidHex()},
		"Sec-MS-GEC":         {edgeGec()},
		"Sec-MS-GEC-Version": {edgeGecVersion},
	}.Encode()
}

// edgeSynthesize 通过 Edge TTS WebSocket 合成语音，返回 MP3 字节
func edgeSynthesize(text, voice, rate, pitch, volume string) ([]byte, error) {
	headers := http.Header{
		"Pragma":              {"no-cache"},
		"Cache-Control":       {"no-cache"},
		"Origin":              {"chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold"},
		"Sec-WebSocket-Version": {"13"},
		"User-Agent":          {edgeUserAgent},
		"Accept-Encoding":     {"gzip, deflate, br, zstd"},
		"Accept-Language":     {"en-US,en;q=0.9"},
		"Cookie":              {"muid=" + edgeMuid() + ";"},
	}

	dialer := websocket.Dialer{
		HandshakeTimeout:   15 * time.Second,
		EnableCompression:  true,
	}
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
	if err := conn.WriteMessage(websocket.TextMessage, []byte(edgeSSMLMessage(edgeCleanText(text), voice, rate, pitch, volume))); err != nil {
		return nil, fmt.Errorf("发送 SSML 失败: %w", err)
	}

	var audio []byte
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if len(audio) > 0 {
				break // 音频已收完，连接关闭可接受
			}
			return nil, fmt.Errorf("Edge TTS 读取失败: %w", err)
		}
		switch msgType {
		case websocket.TextMessage:
			// 文本消息：headers + 空行 + body；Path 参数标识消息类型
			path := edgeTextPath(data)
			if path == "turn.end" {
				goto done
			}
			if path != "" && path != "response" && path != "turn.start" && path != "audio.metadata" {
				return nil, fmt.Errorf("Edge TTS 未知响应: %s", path)
			}
		case websocket.BinaryMessage:
			// 二进制消息：前 2 字节 = header 长度，其后为 headers + 音频数据
			if len(data) < 2 {
				continue
			}
			headerLen := int(binary.BigEndian.Uint16(data[:2]))
			if len(data) < headerLen+2 {
				continue
			}
			audio = append(audio, data[headerLen+2:]...)
		}
	}
done:
	if len(audio) == 0 {
		return nil, fmt.Errorf("Edge TTS 未返回音频")
	}
	return audio, nil
}

// edgeTextPath 从文本消息中解析 Path 头（headers 以 \r\n\r\n 与 body 分隔）
func edgeTextPath(data []byte) string {
	idx := indexDoubleCRLF(data)
	if idx < 0 {
		return ""
	}
	for _, line := range strings.Split(string(data[:idx]), "\r\n") {
		if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "Path" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func indexDoubleCRLF(data []byte) int {
	for i := 0; i+3 < len(data); i++ {
		if data[i] == '\r' && data[i+1] == '\n' && data[i+2] == '\r' && data[i+3] == '\n' {
			return i
		}
	}
	return -1
}

// speakViaEdge 合成语音：指定音色则精确使用（失败即报错）；
// 未指定则尝试默认候选列表
func speakViaEdge(text, voice, rate, pitch, volume string) (*speakResult, error) {
	if voice != "" {
		data, err := edgeSynthesize(text, voice, rate, pitch, volume)
		if err != nil {
			return nil, err
		}
		return &speakResult{
			Audio:       base64.StdEncoding.EncodeToString(data),
			Format:      "mp3",
			DurationSec: estimateDuration(len(data), "mp3"),
		}, nil
	}
	var lastErr error
	for _, v := range edgeVoices {
		data, err := edgeSynthesize(text, v, rate, pitch, volume)
		if err == nil {
			return &speakResult{
				Audio:       base64.StdEncoding.EncodeToString(data),
				Format:      "mp3",
				DurationSec: estimateDuration(len(data), "mp3"),
			}, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("Edge TTS 全部音色失败，最后错误: %v", lastErr)
}
