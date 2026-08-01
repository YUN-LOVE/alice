package server

import (
	"encoding/json"
	"net/http"
	"strconv"

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
