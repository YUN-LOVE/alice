package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPClient MCP HTTP/SSE 客户端：远程 MCP Server 通过 URL 接入
// 支持 Streamable HTTP（application/json 或 text/event-stream 响应）
type HTTPClient struct {
	url        string
	headers    map[string]string
	httpClient *http.Client
	serverInfo ServerInfo
	tools      []Tool
}

func (h *HTTPClient) Start(ctx context.Context, cfg ClientConfig) error {
	h.url = cfg.URL
	h.headers = cfg.Headers
	h.httpClient = &http.Client{Timeout: 30 * time.Second}

	// initialize 握手
	var res initializeResult
	if err := h.post(ctx, "initialize", initializeParams{
		ProtocolVersion: protocolVersion,
		Capabilities:    map[string]any{"tools": map[string]any{}},
		ClientInfo:      ServerInfo{Name: "alice", Version: "0.1.0"},
	}, &res); err != nil {
		return fmt.Errorf("MCP initialize 失败: %w", err)
	}
	h.serverInfo = res.ServerInfo

	// initialized 通知
	_ = h.post(ctx, "notifications/initialized", map[string]any{}, nil)

	// 拉取工具列表
	var tl toolsListResult
	if err := h.post(ctx, "tools/list", map[string]any{}, &tl); err != nil {
		return fmt.Errorf("MCP tools/list 失败: %w", err)
	}
	h.tools = tl.Tools
	return nil
}

// post 发送 JSON-RPC 请求并解析响应（JSON 或 SSE）
func (h *HTTPClient) post(ctx context.Context, method string, params any, out any) error {
	reqBody := rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted {
		return fmt.Errorf("MCP 服务器返回 202（异步响应暂不支持）")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("MCP 返回 %d: %s", resp.StatusCode, string(body))
	}

	ct := resp.Header.Get("Content-Type")
	var rpcResp rpcResponse
	switch {
	case strings.Contains(ct, "text/event-stream"):
		if err := parseSSEResponse(resp.Body, &rpcResp); err != nil {
			return err
		}
	default:
		if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
			return err
		}
	}

	if rpcResp.Error != nil {
		return fmt.Errorf("MCP %s 错误: %s", method, rpcResp.Error.Message)
	}
	if out != nil {
		return json.Unmarshal(rpcResp.Result, out)
	}
	return nil
}

// parseSSEResponse 解析 SSE 流，取出第一条 data 消息
func parseSSEResponse(r io.Reader, out *rpcResponse) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var msg rpcMessage
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			continue
		}
		if msg.ID != nil {
			out.ID = *msg.ID
		}
		out.Result = msg.Result
		out.Error = msg.Error
		if out.Error != nil || out.Result != nil {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("SSE 响应中无有效消息")
}

func (h *HTTPClient) Tools() []Tool { return h.tools }

func (h *HTTPClient) ServerInfo() ServerInfo { return h.serverInfo }

// Call 调用工具
func (h *HTTPClient) Call(ctx context.Context, name string, args map[string]any) (*CallResult, error) {
	var res CallResult
	if err := h.post(ctx, "tools/call", toolsCallParams{Name: name, Arguments: args}, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (h *HTTPClient) Close() {}
