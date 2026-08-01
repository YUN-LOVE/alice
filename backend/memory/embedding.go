package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// Embedder 文本向量化接口
type Embedder interface {
	// Embed 返回文本的向量（归一化）
	Embed(ctx context.Context, text string) ([]float64, error)
	Name() string
}

// NewEmbedder 创建 Embedder；未配 Key 时返回 hash 伪向量（保证链路可测，语义不保证）
func NewEmbedder(provider, baseURL, apiKey, model string) Embedder {
	if provider == "mock" || apiKey == "" {
		return &HashEmbedder{model: model}
	}
	return &OpenAIEmbedder{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ==================== OpenAI 兼容实现（硅基流动等） ====================

type OpenAIEmbedder struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func (e *OpenAIEmbedder) Name() string { return e.model }

func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	body, err := json.Marshal(map[string]any{
		"model": e.model,
		"input": text,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("embedding 返回 %d: %s", resp.StatusCode, string(data))
	}

	var out struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, fmt.Errorf("embedding 返回空数据")
	}
	return Normalize(out.Data[0].Embedding), nil
}

// ==================== Hash 兜底（无 Key 开发模式） ====================

const hashDim = 256

type HashEmbedder struct {
	model string
}

func (h *HashEmbedder) Name() string { return h.model + " (hash)" }

// Embed 用 n-gram hash 生成确定性伪向量：相同文本→相同向量，词面相似→向量相近
// 检索链路可测试，但无语义能力；配 Key 后自动切换真实 Embedding
func (h *HashEmbedder) Embed(_ context.Context, text string) ([]float64, error) {
	vec := make([]float64, hashDim)
	runes := []rune(strings.ToLower(text))
	for i := 0; i < len(runes); i++ {
		var gram int
		for j := 0; j < 3 && i+j < len(runes); j++ {
			gram = gram*131 + int(runes[i+j])
		}
		idx := int(fnv32(uint32(gram))) % hashDim
		vec[idx]++
	}
	return Normalize(vec), nil
}

func fnv32(v uint32) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
	return h.Sum32()
}

// ==================== 向量工具 ====================

// Normalize L2 归一化（余弦相似度 = 点积）
func Normalize(v []float64) []float64 {
	var sum float64
	for _, x := range v {
		sum += x * x
	}
	norm := math.Sqrt(sum)
	if norm == 0 {
		return v
	}
	for i := range v {
		v[i] /= norm
	}
	return v
}

// Cosine 计算两个归一化向量的余弦相似度
func Cosine(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot float64
	for i := range a {
		dot += a[i] * b[i]
	}
	return dot
}
