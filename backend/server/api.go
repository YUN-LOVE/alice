package server

import (
	"strings"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

	// 当前可调设置（LLM / 情绪 / 记忆容量），设置面板加载用
	mux.HandleFunc("GET /api/v1/settings", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, k.Settings())
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

	// 当前情绪状态（面板加载用，避免等待下一次对话的 emotion_update）
	mux.HandleFunc("GET /api/v1/emotion/state", func(w http.ResponseWriter, r *http.Request) {
		state := k.Emotion().State()
		desc, _, top := k.Emotion().Summary()
		writeJSON(w, http.StatusOK, map[string]any{"state": state, "description": desc, "top": top})
	})

	// 主动推送开关（查询 / 切换）
	mux.HandleFunc("GET /api/v1/emotion/proactive", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": k.ProactiveEnabled()})
	})
	mux.HandleFunc("POST /api/v1/emotion/proactive", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"message": "请求格式错误"})
			return
		}
		k.SetProactiveEnabled(body.Enabled)
		writeJSON(w, http.StatusOK, map[string]any{"enabled": body.Enabled})
	})

	// 语音转文字（STT）：multipart 上传音频 → {text}
	// 前端录音（MediaRecorder）后上传，后端调内部 STT MCP Server 转写
	mux.HandleFunc("POST /api/v1/audio/stt", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"message": "解析上传失败: " + err.Error()})
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"message": "缺少音频文件字段 (file)"})
			return
		}
		defer file.Close()

		ext := filepath.Ext(filepath.Base(header.Filename))
		tmp, err := os.CreateTemp(filepath.Join(kernel.UploadsDir(), "tmp"), "stt-*"+ext)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": "创建临时文件失败: " + err.Error()})
			return
		}
		defer os.Remove(tmp.Name())
		if _, err := io.Copy(tmp, file); err != nil {
			tmp.Close()
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": "保存音频失败: " + err.Error()})
			return
		}
		if err := tmp.Close(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": "保存音频失败: " + err.Error()})
			return
		}

		text, err := k.STT(r.Context(), tmp.Name())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": "语音识别失败: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"text": text})
	})

	// TTS 可用音色列表（设置面板加载用，?locale=zh 可选）
	mux.HandleFunc("GET /api/v1/audio/voices", func(w http.ResponseWriter, r *http.Request) {
		voices, err := k.AudioVoices(r.Context(), r.URL.Query().Get("locale"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": "获取音色列表失败: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"voices": voices})
	})

	// TTS 合成（设置面板试听/通用）：{text, voice?, rate?, pitch?, volume?} → {url, duration_sec}
	mux.HandleFunc("POST /api/v1/audio/tts", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text   string `json:"text"`
			Voice  string `json:"voice"`
			Rate   string `json:"rate"`
			Pitch  string `json:"pitch"`
			Volume string `json:"volume"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"message": "请求格式错误"})
			return
		}
		if strings.TrimSpace(body.Text) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"message": "缺少 text"})
			return
		}
		params := map[string]any{
			"voice":  body.Voice,
			"rate":   body.Rate,
			"pitch":  body.Pitch,
			"volume": body.Volume,
		}
		urlPath, dur, err := k.TTS(r.Context(), body.Text, params)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": "TTS 合成失败: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"url": urlPath, "duration_sec": dur})
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
