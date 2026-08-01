package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ChatMessage 对话消息
type ChatMessage struct {
	Role    string `json:"role"` // system / user / assistant / tool
	Content string `json:"content"`
}

// Tool 工具定义（Function Calling 预留，阶段四启用）
type Tool struct {
	Type     string         `json:"type"`
	Function ToolDefinition `json:"function"`
}

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall LLM 请求调用的工具（阶段四启用）
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// StreamChunk 流式回复片段
type StreamChunk struct {
	Content   string
	ToolCalls []ToolCall
	Done      bool
}

// LLMClient 大模型客户端接口
type LLMClient interface {
	// ChatStream 流式对话，返回逐字片段 channel
	ChatStream(ctx context.Context, messages []ChatMessage, tools []Tool) (<-chan StreamChunk, error)
	Name() string
}

// NewLLMClient 根据配置创建客户端；未配 API Key 或 provider=mock 时返回 Mock
func NewLLMClient(provider, baseURL, apiKey, model string, temperature float64, maxTokens int) LLMClient {
	if provider == "mock" || apiKey == "" {
		return &MockClient{model: model}
	}
	return &OpenAIClient{
		baseURL:     strings.TrimSuffix(baseURL, "/"),
		apiKey:      apiKey,
		model:       model,
		temperature: temperature,
		maxTokens:   maxTokens,
		httpClient:  &http.Client{Timeout: 120 * time.Second},
	}
}

// ==================== OpenAI 兼容实现 ====================

type OpenAIClient struct {
	baseURL     string
	apiKey      string
	model       string
	temperature float64
	maxTokens   int
	httpClient  *http.Client
}

func (c *OpenAIClient) Name() string { return c.model }

func (c *OpenAIClient) ChatStream(ctx context.Context, messages []ChatMessage, tools []Tool) (<-chan StreamChunk, error) {
	body := map[string]any{
		"model":       c.model,
		"messages":    messages,
		"temperature": c.temperature,
		"max_tokens":  c.maxTokens,
		"stream":      true,
	}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", strings.NewReader(string(payload)))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 LLM 失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("LLM 返回 %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan StreamChunk, 64)
	go c.streamLoop(ctx, resp.Body, ch)
	return ch, nil
}

type sseLine struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

func (c *OpenAIClient) streamLoop(ctx context.Context, body io.ReadCloser, ch chan<- StreamChunk) {
	defer body.Close()
	defer close(ch)

	for {
		line, err := readSSELine(body)
		if err != nil {
			if err == io.EOF {
				return
			}
			if ctx.Err() != nil {
				return
			}
			return
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			ch <- StreamChunk{Done: true}
			return
		}
		var s sseLine
		if err := json.Unmarshal([]byte(data), &s); err != nil {
			continue
		}
		if len(s.Choices) == 0 {
			continue
		}
		delta := s.Choices[0].Delta
		chunk := StreamChunk{Content: delta.Content}
		for _, tc := range delta.ToolCalls {
			chunk.ToolCalls = append(chunk.ToolCalls, ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
		ch <- chunk
	}
}

// readSSELine 按 SSE 格式逐行读取
func readSSELine(r io.Reader) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				line := sb.String()
				if strings.HasSuffix(line, "\r") {
					line = line[:len(line)-1]
				}
				return line, nil
			}
			sb.WriteByte(buf[0])
		}
		if err != nil {
			if sb.Len() > 0 {
				return sb.String(), err
			}
			return "", err
		}
	}
}

// ==================== Mock 实现（未配 Key 时兜底） ====================

type MockClient struct {
	model string
}

func (m *MockClient) Name() string { return m.model + " (mock)" }

// mockReplies 根据用户输入选择回复模板，让开发期对话看起来有生气
func mockReply(messages []ChatMessage) string {
	var lastUser string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUser = messages[i].Content
			break
		}
	}

	replies := []string{
		"（模拟回复）我听到你说的了：「" + lastUser + "」听起来挺有意思的。等你在 kernel.yaml 里配好 API Key，我就能真正读懂你啦。",
		"（模拟回复）「" + lastUser + "」——我记下了。现在是 mock 模式，我只会复读。配好 DeepSeek 或硅基流动的 key 之后，我就能认真陪你聊天了。",
		"（模拟回复）嗯，「" + lastUser + "」我在想怎么接。开发期我先假装听懂，正式上线换我！",
	}
	idx := time.Now().UnixNano() % int64(len(replies))
	return replies[idx]
}

func (m *MockClient) ChatStream(ctx context.Context, messages []ChatMessage, _ []Tool) (<-chan StreamChunk, error) {
	reply := mockReply(messages)
	ch := make(chan StreamChunk, 64)
	go func() {
		defer close(ch)
		for _, r := range []rune(reply) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(15 * time.Millisecond):
				ch <- StreamChunk{Content: string(r)}
			}
		}
		ch <- StreamChunk{Done: true}
	}()
	return ch, nil
}
