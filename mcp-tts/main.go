// TTS MCP Server：让 Alice 能"说话"
// 工具：
//   speak  { text: 文本, voice?: 音色 } → {"audio":"<base64>","format":"mp3","duration_sec":x}
//
// 引擎优先级（环境变量，密钥不进仓库）：
//   1. ALICE_TTS_API_KEY   配置 → OpenAI 兼容 /audio/speech（硅基流动 CosyVoice 等）
//      ALICE_TTS_BASE_URL  默认 https://api.siliconflow.cn/v1
//      ALICE_TTS_MODEL     默认 FunAudioLLM/CosyVoice2-0.5B
//      ALICE_TTS_VOICE     默认 female-calm
//   2. 未配置 → Edge TTS 免费接口（微软，无需 key，依赖访问微软网络）
//      ALICE_TTS_EDGE_VOICE 默认 zh-CN-XiaoxiaoNeural（失败自动尝试备用音色）
//
// 返回 base64 音频由调用方解码保存（MCP 文本通道不适合直接传二进制文件）。
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const protocolVersion = "2024-11-05"

type msg struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      *int64    `json:"id,omitempty"`
	Method  string    `json:"method,omitempty"`
	Params  any       `json:"params,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// speakResult 工具返回的 JSON 文本
type speakResult struct {
	Audio       string  `json:"audio"`        // base64 编码的音频
	Format      string  `json:"format"`       // mp3 / wav
	DurationSec float64 `json:"duration_sec"` // 估算时长（秒）
}

// speakArgs 工具参数（voice/rate/pitch/volume 均为可选，Edge TTS 模式生效）
type speakArgs struct {
	Text   string `json:"text"`
	Voice  string `json:"voice"`
	Rate   string `json:"rate"`
	Pitch  string `json:"pitch"`
	Volume string `json:"volume"`
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	enc := json.NewEncoder(os.Stdout)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Fprintln(os.Stderr, "读取失败:", err)
			}
			return
		}
		if line == "\n" || line == "" {
			continue
		}
		var m msg
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}
		handle(&m, enc)
	}
}

func handle(m *msg, enc *json.Encoder) {
	switch m.Method {
	case "initialize":
		respond(enc, m.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "alice-tts", "version": "0.1.0"},
		})

	case "notifications/initialized":
		// 通知无需响应

	case "ping":
		respond(enc, m.ID, map[string]any{})

	case "tools/list":
		respond(enc, m.ID, map[string]any{
			"tools": []any{
				map[string]any{
					"name":        "speak",
					"description": "把文本合成语音，返回 base64 音频。用于 Alice 说话（TTS）。voice/rate/pitch/volume 为可选参数（Edge TTS 模式生效，缺省用环境变量默认值）。",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"text":   map[string]any{"type": "string", "description": "要朗读的文本"},
							"voice":  map[string]any{"type": "string", "description": "音色名，如 zh-CN-XiaoyiNeural / en-US-EmmaMultilingualNeural"},
							"rate":   map[string]any{"type": "string", "description": "语速，如 +10% / -20%"},
							"pitch":  map[string]any{"type": "string", "description": "音高，如 +5Hz / -3Hz"},
							"volume": map[string]any{"type": "string", "description": "音量，如 +10% / -50%"},
						},
						"required": []string{"text"},
					},
				},
			},
		})

	case "tools/call":
		params, _ := m.Params.(map[string]any)
		name, _ := params["name"].(string)
		args, _ := params["arguments"].(map[string]any)
		switch name {
		case "speak":
			sa := speakArgs{}
			if v, ok := args["text"].(string); ok {
				sa.Text = v
			}
			if v, ok := args["voice"].(string); ok {
				sa.Voice = v
			}
			if v, ok := args["rate"].(string); ok {
				sa.Rate = v
			}
			if v, ok := args["pitch"].(string); ok {
				sa.Pitch = v
			}
			if v, ok := args["volume"].(string); ok {
				sa.Volume = v
			}
			out, err := speak(sa)
			if err != nil {
				respond(enc, m.ID, map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "TTS 失败: " + err.Error()}},
					"isError": true,
				})
				return
			}
			data, _ := json.Marshal(out)
			respond(enc, m.ID, map[string]any{
				"content": []any{map[string]any{"type": "text", "text": string(data)}},
				"isError": false,
			})
		default:
			respondErr(enc, m.ID, -32602, "未知工具: "+name)
		}

	default:
		respondErr(enc, m.ID, -32601, "未知方法: "+m.Method)
	}
}

func respond(enc *json.Encoder, id *int64, result any) {
	_ = enc.Encode(msg{JSONRPC: "2.0", ID: id, Result: result})
}

func respondErr(enc *json.Encoder, id *int64, code int, message string) {
	_ = enc.Encode(msg{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}

// speak 合成语音：API 优先，无 key 降级 Edge TTS（支持音色/语速/音高/音量）
func speak(sa speakArgs) (*speakResult, error) {
	text := strings.TrimSpace(sa.Text)
	if text == "" {
		return nil, fmt.Errorf("文本为空")
	}
	// 保护：超长文本截断（单次合成长度限制）
	const maxRunes = 2000
	if r := []rune(text); len(r) > maxRunes {
		text = string(r[:maxRunes]) + "…"
	}

	if os.Getenv("ALICE_TTS_API_KEY") != "" {
		return speakViaAPI(text, sa.Voice)
	}
	p := edgeParams(edgeTTSParams{
		Voice:  sa.Voice,
		Rate:   sa.Rate,
		Pitch:  sa.Pitch,
		Volume: sa.Volume,
	})
	return speakViaEdge(text, p.Voice, p.Rate, p.Pitch, p.Volume)
}
