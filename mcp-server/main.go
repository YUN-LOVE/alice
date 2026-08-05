// 内置测试 MCP Server：供 Alice 开发测试使用（stdio 传输）
// 工具：calculator（四则运算）、get_time、echo
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const version = "2024-11-05"

type msg struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"`
	Method  string `json:"method,omitempty"`
	Params  any    `json:"params,omitempty"`
	Result  any    `json:"result,omitempty"`
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
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "alice-local-tools", "version": "0.1.0"},
		})

	case "notifications/initialized":
		// 通知无需响应

	case "tools/list":
		respond(enc, m.ID, map[string]any{
			"tools": []any{
				map[string]any{
					"name":        "calculator",
					"description": "计算四则运算表达式，如 \"(1 + 2) * 3\"",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"expression": map[string]any{"type": "string", "description": "数学表达式"},
						},
						"required": []string{"expression"},
					},
				},
				map[string]any{
					"name":        "get_time",
					"description": "获取当前日期时间",
					"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
				},
				map[string]any{
					"name":        "echo",
					"description": "原样返回输入的文本",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"text": map[string]any{"type": "string"},
						},
						"required": []string{"text"},
					},
				},
			},
		})

	case "tools/call":
		params := m.Params.(map[string]any)
		name, _ := params["name"].(string)
		args, _ := params["arguments"].(map[string]any)
		content := callTool(name, args)
		respond(enc, m.ID, map[string]any{
			"content": []any{map[string]any{"type": "text", "text": content}},
			"isError": false,
		})

	case "ping":
		respond(enc, m.ID, map[string]any{})

	default:
		respondErr(enc, m.ID, -32601, "未知方法: "+m.Method)
	}
}

func callTool(name string, args map[string]any) string {
	switch name {
	case "calculator":
		expr, _ := args["expression"].(string)
		v, err := eval(expr)
		if err != nil {
			return "计算失败: " + err.Error()
		}
		return fmt.Sprintf("%s = %v", expr, v)
	case "get_time":
		return time.Now().Format("2006-01-02 15:04:05")
	case "echo":
		t, _ := args["text"].(string)
		return t
	default:
		return "未知工具: " + name
	}
}

func respond(enc *json.Encoder, id *int64, result any) {
	_ = enc.Encode(msg{JSONRPC: "2.0", ID: id, Result: result})
}

func respondErr(enc *json.Encoder, id *int64, code int, message string) {
	_ = enc.Encode(msg{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}

// ==================== 简单四则运算解析器 ====================

func eval(expr string) (float64, error) {
	p := &parser{tokens: tokenize(expr)}
	v, err := p.expr()
	if err != nil {
		return 0, err
	}
	return v, nil
}

func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n':
			flush()
		case '+', '-', '*', '/', '(', ')':
			flush()
			out = append(out, string(r))
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

type parser struct {
	tokens []string
	pos    int
}

func (p *parser) peek() string {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return ""
}

func (p *parser) next() string {
	t := p.peek()
	p.pos++
	return t
}

func (p *parser) expr() (float64, error) {
	v, err := p.term()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek() {
		case "+":
			p.next()
			r, err := p.term()
			if err != nil {
				return 0, err
			}
			v += r
		case "-":
			p.next()
			r, err := p.term()
			if err != nil {
				return 0, err
			}
			v -= r
		default:
			return v, nil
		}
	}
}

func (p *parser) term() (float64, error) {
	v, err := p.factor()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek() {
		case "*":
			p.next()
			r, err := p.factor()
			if err != nil {
				return 0, err
			}
			v *= r
		case "/":
			p.next()
			r, err := p.factor()
			if err != nil {
				return 0, err
			}
			if r == 0 {
				return 0, fmt.Errorf("除零")
			}
			v /= r
		default:
			return v, nil
		}
	}
}

func (p *parser) factor() (float64, error) {
	t := p.peek()
	if t == "-" {
		p.next()
		v, err := p.factor()
		return -v, err
	}
	if t == "(" {
		p.next()
		v, err := p.expr()
		if err != nil {
			return 0, err
		}
		if p.next() != ")" {
			return 0, fmt.Errorf("括号不匹配")
		}
		return v, nil
	}
	return strconv.ParseFloat(p.next(), 64)
}
