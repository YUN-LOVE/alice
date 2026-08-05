package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"alice/kernel"
)

// RegisterRoutes 注册 HTTP API 路由
func RegisterRoutes(mux *http.ServeMux, k *kernel.Kernel) {
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"llm":       k.LLMName(),
			"embedding": k.EmbedderName(),
			"memory":    memoryCount(w, r, k),
		})
	})

	// Memory Block 只读查看
	mux.HandleFunc("GET /api/v1/memory/block", func(w http.ResponseWriter, r *http.Request) {
		entries := k.BlockList()
		writeJSON(w, http.StatusOK, map[string]any{
			"total":   len(entries),
			"entries": entries,
		})
	})

	// 单条记忆详情
	mux.HandleFunc("GET /api/v1/memory/block/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"message": "无效的 id"})
			return
		}
		entry, ok := k.BlockGet(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"message": "记忆不存在"})
			return
		}
		writeJSON(w, http.StatusOK, entry)
	})

	// RAG 历史搜索
	mux.HandleFunc("POST /api/v1/memory/search", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"message": "请求格式错误"})
			return
		}
		results, err := k.MemorySearch(r.Context(), body.Query)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": "检索失败: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": results})
	})

	// 导出所有记忆（JSON 下载）
	mux.HandleFunc("GET /api/v1/memory/export", func(w http.ResponseWriter, r *http.Request) {
		mems, err := k.MemoryExport(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": "导出失败: " + err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="alice-memory.json"`)
		_ = json.NewEncoder(w).Encode(map[string]any{"total": len(mems), "memories": mems})
	})

	// 历史聊天记录：按日期返回当天对话（每天零点整理归档，之前的日子在 RAG 中）
	mux.HandleFunc("GET /api/v1/history", func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		mems, err := k.History(r.Context(), date)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": "加载历史失败: " + err.Error()})
			return
		}
		messages := make([]map[string]any, 0, len(mems))
		for _, m := range mems {
			messages = append(messages, map[string]any{
				"role":      m.Role,
				"content":   m.Text,
				"create_at": m.CreateAt.Unix(),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"date": date, "messages": messages})
	})

	// 已归档日期列表（前端可选展示）
	mux.HandleFunc("GET /api/v1/history/dates", func(w http.ResponseWriter, r *http.Request) {
		dates, err := k.MemoryDates(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": "获取日期失败: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"dates": dates})
	})

	// MCP Server 状态
	mux.HandleFunc("GET /api/v1/mcp/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"servers": k.MCPStatus()})
	})

	// MCP 市场（注册表可安装项）
	mux.HandleFunc("GET /api/v1/mcp/market", func(w http.ResponseWriter, r *http.Request) {
		items := []map[string]any{}
		if reg := k.MCPRegistry(); reg != nil {
			installed := make(map[string]bool)
			for _, s := range k.MCPStatus() {
				installed[s.ID] = true
			}
			for _, it := range reg.Servers {
				items = append(items, map[string]any{
					"id": it.ID, "name": it.Name, "description": it.Description,
					"installed": installed[it.ID],
				})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})

	// 情绪显著事件记录
	mux.HandleFunc("GET /api/v1/emotion/events", func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				limit = n
			}
		}
		events, err := k.EmotionEvents(r.Context(), limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": "查询情绪事件失败: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"events": events})
	})
}

func memoryCount(w http.ResponseWriter, r *http.Request, k *kernel.Kernel) int64 {
	n, err := k.MemoryCount(r.Context())
	if err != nil {
		return 0
	}
	return n
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
