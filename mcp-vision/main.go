// Vision MCP Server：让 Alice 能"看"图像
// 工具：
//   describe_image        { path: 本地图片路径 } → 视觉模型描述图片内容
//   describe_image_url    { url: 图片URL }       → 视觉模型描述图片内容
//
// 视觉模型配置（环境变量，避免密钥进仓库）：
//   ALICE_VISION_API_KEY  必填，硅基流动/OpenAI 兼容 key
//   ALICE_VISION_BASE_URL 默认 https://api.siliconflow.cn/v1
//   ALICE_VISION_MODEL    默认 Qwen/Qwen2.5-VL-72B-Instruct
//   ALICE_LLM_API_KEY     当未单独配置视觉 key 时兜底
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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

var (
	visionKey     = os.Getenv("ALICE_VISION_API_KEY")
	visionBaseURL = envOr("ALICE_VISION_BASE_URL", "https://api.siliconflow.cn/v1")
	visionModel   = envOr("ALICE_VISION_MODEL", "Qwen/Qwen2.5-VL-72B-Instruct")
	httpClient    = &http.Client{Timeout: 60 * time.Second}
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	if visionKey == "" {
		visionKey = os.Getenv("ALICE_LLM_API_KEY")
	}
	if visionKey == "" {
		fmt.Fprintln(os.Stderr, "警告: 未配置 ALICE_VISION_API_KEY，describe_image 将返回提示（开发模式）")
	}

	reader := bufio.NewReader(os.Stdin)
	enc := json.NewEncoder(os.Stdout)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
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
			"serverInfo":      map[string]any{"name": "alice-vision", "version": "0.1.0"},
		})
	case "notifications/initialized", "ping":
		if m.Method == "ping" {
			respond(enc, m.ID, map[string]any{})
		}
	case "tools/list":
		respond(enc, m.ID, map[string]any{
			"tools": []any{
				map[string]any{
					"name":        "describe_image",
					"description": "描述一张本地图片的内容（支持 jpg/png/webp）。传入服务器可访问的图片路径。",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{"type": "string", "description": "本地图片文件的绝对路径"},
						},
						"required": []string{"path"},
					},
				},
				map[string]any{
					"name":        "describe_image_url",
					"description": "描述一个网络图片 URL 的内容。",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"url": map[string]any{"type": "string", "description": "图片的 http(s) URL"},
						},
						"required": []string{"url"},
					},
				},
			},
		})
	case "tools/call":
		params, _ := m.Params.(map[string]any)
		name, _ := params["name"].(string)
		args, _ := params["arguments"].(map[string]any)
		text, err := callTool(name, args)
		if err != nil {
			respond(enc, m.ID, map[string]any{
				"content": []any{map[string]any{"type": "text", "text": "工具调用失败: " + err.Error()}},
				"isError": true,
			})
			return
		}
		respond(enc, m.ID, map[string]any{
			"content": []any{map[string]any{"type": "text", "text": text}},
			"isError": false,
		})
	default:
		respondErr(enc, m.ID, -32601, "未知方法: "+m.Method)
	}
}

func callTool(name string, args map[string]any) (string, error) {
	if visionKey == "" {
		return "（开发模式）视觉模型未配置 key，无法描述图片。请配置 ALICE_VISION_API_KEY 后重启。", nil
	}
	switch name {
	case "describe_image":
		path, _ := args["path"].(string)
		if path == "" {
			return "", fmt.Errorf("缺少图片路径")
		}
		return describeImageFile(path)
	case "describe_image_url":
		url, _ := args["url"].(string)
		if url == "" {
			return "", fmt.Errorf("缺少图片 URL")
		}
		return describeImageURL(url)
	default:
		return "", fmt.Errorf("未知工具: %s", name)
	}
}

func describeImageFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取图片失败: %w", err)
	}
	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if mimeType == "" {
		mimeType = "image/png"
	}
	dataURI := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
	return callVision(dataURI, filepath.Base(path))
}

func describeImageURL(url string) (string, error) {
	return callVision(url, url)
}

func callVision(imageRef, label string) (string, error) {
	payload := map[string]any{
		"model": visionModel,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "请用中文详细描述这张图片的内容，包括主体、场景、文字和氛围。"},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageRef}},
				},
			},
		},
		"max_tokens": 1024,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, strings.TrimSuffix(visionBaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+visionKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("视觉模型返回 %d: %s", resp.StatusCode, string(data))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("视觉模型无输出")
	}
	return "图片（" + label + "）的描述：\n" + out.Choices[0].Message.Content, nil
}

func respond(enc *json.Encoder, id *int64, result any) {
	_ = enc.Encode(msg{JSONRPC: "2.0", ID: id, Result: result})
}

func respondErr(enc *json.Encoder, id *int64, code int, message string) {
	_ = enc.Encode(msg{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}
