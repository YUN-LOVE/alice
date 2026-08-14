// STT MCP Server：让 Alice 能"听到"用户说话
// 工具：
//   transcribe  { audio_path: 音频文件路径 } 或 { audio_base64: 音频内容, format: 扩展名 } → {"text":"..."}
//
// 引擎优先级（环境变量，密钥不进仓库）：
//   1. ALICE_STT_API_KEY  配置 → OpenAI 兼容 /audio/transcriptions（硅基流动 SenseVoice 等）
//      ALICE_STT_BASE_URL 默认 https://api.siliconflow.cn/v1
//      ALICE_STT_MODEL    默认 iic/SenseVoiceSmall
//   2. 未配置 → 本地 whisper.cpp CLI（whisper-cli / main）
//      ALICE_WHISPER_BIN   默认 whisper-cli（可指向 whisper.cpp 编译产物）
//      ALICE_WHISPER_MODEL 默认 models/ggml-base.bin
//
// 注意：本 Server 的工具有内部属性（mcp.yaml internal: true），
// 不暴露给 LLM 工具列表，只由后端直接调用（音频数据不应进 LLM 上下文）。
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
			"serverInfo":      map[string]any{"name": "alice-stt", "version": "0.1.0"},
		})

	case "notifications/initialized":
		// 通知无需响应

	case "ping":
		respond(enc, m.ID, map[string]any{})

	case "tools/list":
		respond(enc, m.ID, map[string]any{
			"tools": []any{
				map[string]any{
					"name":        "transcribe",
					"description": "把语音转成文字（STT）。传入本地音频文件路径或 base64 音频内容。",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"audio_path":   map[string]any{"type": "string", "description": "音频文件绝对路径"},
							"audio_base64": map[string]any{"type": "string", "description": "base64 编码的音频内容（与 audio_path 二选一）"},
							"format":       map[string]any{"type": "string", "description": "音频扩展名，如 webm/mp3/wav（base64 方式时使用）"},
						},
					},
				},
			},
		})

	case "tools/call":
		params, _ := m.Params.(map[string]any)
		name, _ := params["name"].(string)
		args, _ := params["arguments"].(map[string]any)
		switch name {
		case "transcribe":
			text, err := transcribe(args)
			if err != nil {
				respond(enc, m.ID, map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "STT 失败: " + err.Error()}},
					"isError": true,
				})
				return
			}
			data, _ := json.Marshal(map[string]string{"text": text})
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

// transcribe 语音转文字：API 优先，无 key 降级本地 whisper
func transcribe(args map[string]any) (string, error) {
	path, _ := args["audio_path"].(string)
	b64, _ := args["audio_base64"].(string)
	format, _ := args["format"].(string)

	if path == "" && b64 == "" {
		return "", fmt.Errorf("需要 audio_path 或 audio_base64")
	}

	// 写临时文件（本地 whisper 需要文件路径）
	if path == "" {
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return "", fmt.Errorf("base64 解码失败: %w", err)
		}
		f, err := os.CreateTemp("", "alice-stt-*."+sanitizeExt(format))
		if err != nil {
			return "", err
		}
		defer os.Remove(f.Name())
		if _, err := f.Write(data); err != nil {
			f.Close()
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		path = f.Name()
	}

	if os.Getenv("ALICE_STT_API_KEY") != "" {
		return transcribeViaAPI(path)
	}
	return transcribeViaWhisper(path)
}

func sanitizeExt(format string) string {
	if format == "" {
		return "webm"
	}
	out := ""
	for _, r := range format {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out += string(r)
		}
	}
	if out == "" {
		return "webm"
	}
	return out
}

func respond(enc *json.Encoder, id *int64, result any) {
	_ = enc.Encode(msg{JSONRPC: "2.0", ID: id, Result: result})
}

func respondErr(enc *json.Encoder, id *int64, code int, message string) {
	_ = enc.Encode(msg{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}
