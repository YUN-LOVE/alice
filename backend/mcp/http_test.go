package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"alice/config"
)

// mockHTTPMCP 模拟远程 MCP Server（返回 JSON 响应）
func mockHTTPMCP(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: mustJSON(map[string]any{
				"protocolVersion": protocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "remote-mock", "version": "1.0"},
			})})
		case "tools/list":
			json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: mustJSON(map[string]any{
				"tools": []any{
					map[string]any{
						"name":        "add",
						"description": "两个数相加",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"a": map[string]any{"type": "number"},
								"b": map[string]any{"type": "number"},
							},
							"required": []string{"a", "b"},
						},
					},
				},
			})})
		case "tools/call":
			var params toolsCallParams
			data, _ := json.Marshal(req.Params)
			json.Unmarshal(data, &params)
			var args struct {
				A float64 `json:"a"`
				B float64 `json:"b"`
			}
			data, _ = json.Marshal(params.Arguments)
			json.Unmarshal(data, &args)
			json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: mustJSON(map[string]any{
				"content": []any{map[string]any{"type": "text", "text": "sum: " + jsonNumber(args.A+args.B)}},
				"isError": false,
			})})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		default:
			json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "unknown"}})
		}
	})
	return httptest.NewServer(mux)
}

// TestHTTPClientLifecycle 验证 HTTP transport：握手 → 工具列表 → 调用
func TestHTTPClientLifecycle(t *testing.T) {
	srv := mockHTTPMCP(t)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c := &HTTPClient{}
	if err := c.Start(ctx, ClientConfig{URL: srv.URL + "/mcp"}); err != nil {
		t.Fatalf("HTTP Start 失败: %v", err)
	}
	defer c.Close()

	if c.ServerInfo().Name != "remote-mock" {
		t.Fatalf("ServerInfo 错误: %v", c.ServerInfo())
	}
	tools := c.Tools()
	if len(tools) != 1 || tools[0].Name != "add" {
		t.Fatalf("工具列表错误: %v", tools)
	}

	res, err := c.Call(ctx, "add", map[string]any{"a": 2, "b": 3})
	if err != nil {
		t.Fatalf("Call 失败: %v", err)
	}
	if len(res.Content) == 0 || !contains(res.Content[0].Text, "5") {
		t.Fatalf("Call 结果错误: %+v", res.Content)
	}
}

// TestManagerHTTP 通过 Manager 分发远程 HTTP MCP
func TestManagerHTTP(t *testing.T) {
	srv := mockHTTPMCP(t)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	m := NewManager(true, "", "", 0)
	m.Add("remote", "远程工具", config.MCPServerConfig{
		Transport: "http",
		URL:       srv.URL + "/mcp",
		Enabled:   true,
	})
	m.StartAll(ctx)

	tools := m.Tools()
	if len(tools) != 1 || tools[0].Name != "remote__add" {
		t.Fatalf("Manager HTTP 工具错误: %v", tools)
	}
	result, err := m.Call(ctx, "remote__add", map[string]any{"a": 10, "b": 5})
	if err != nil {
		t.Fatalf("Manager Call 失败: %v", err)
	}
	if !contains(result, "15") {
		t.Fatalf("Manager Call 结果错误: %s", result)
	}
}

func jsonNumber(v float64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
