# Alice — AI 陪伴系统 API 文档

前端与后端完全解耦：客户端只需知道一个 **后端地址**（HTTP 形式，如 `http://192.168.1.5:8081`），WebSocket 与 HTTP 均由它推导：

```
后端地址:      http://host:port
WebSocket:     ws://host:port/ws
HTTP API:      http://host:port/api/v1/...
```

支持跨域访问（CORS `*`），任意多端（Web / 移动 / 桌面 / 脚本）均可接入。

---

## 一、WebSocket 协议（`/ws`）

统一消息结构（JSON Text Frame）：

```json
{ "type": "消息类型", "payload": { ... } }
```

### 1.1 前端 → 后端

| type | payload | 说明 | 状态 |
|------|---------|------|------|
| `handshake` | `{ "session_id": "可选" }` | 握手；绑定会话 | ✅ 已实现 |
| `user_message` | `{ "text": "内容", "session_id": "可选" }` | 发送文本消息 | ✅ 已实现 |
| `ping` | — | 心跳 | ✅ 已实现 |
| `mcp_installed_list` | — | 获取已安装 MCP 列表 | ✅ 已实现 |
| `mcp_toggle` | `{ "id": "serverId", "enabled": true/false }` | 启用/禁用 MCP | ✅ 已实现 |
| `mcp_tool_toggle` | `{ "server", "tool", "enabled" }` | 工具级启用/禁用（持久化） | ✅ 已实现 |
| `mcp_market_list` | — | 获取 MCP 市场列表 | ✅ 已实现 |
| `mcp_install` | `{ "id" }` | 安装 MCP（写入 mcp.yaml + 热重载） | ✅ 已实现 |
| `mcp_uninstall` | `{ "id" }` | 卸载 MCP | ✅ 已实现 |
| `mcp_configure` | `{ "id", "config": { "args", "env", "url", "enabled" } }` | 修改 MCP 配置（写回 mcp.yaml + 热重载） | ✅ 已实现 |
| `settings_update` | `{ "section": "llm"/"emotion"/"block", "values": {...} }` | 修改配置（写回 YAML + 热重载） | ✅ 已实现 |
| `upload_start` | `{ "upload_id", "file_name", "total_chunks" }` | 开始分块上传（随后发送 total_chunks 个 Binary Frame） | ✅ 已实现 |
| `upload_end` | `{ "upload_id" }` | 结束分块上传，校验完整性 | ✅ 已实现 |
| `upload_chunk` | Binary Frame | 上传文件分块（upload_start 之后发送） | ✅ 已实现 |

### 1.2 后端 → 前端

| type | payload | 说明 | 状态 |
|------|---------|------|------|
| `handshake_ack` | `{ "name", "llm", "version", "serverTime", "session_id" }` | 握手确认 | ✅ 已实现 |
| `assistant_chunk` | `{ "content": "增量文本", "done": true/false }` | 流式回复（逐字推送，`done:true` 结束） | ✅ 已实现 |
| `pong` | `{ "time" }` | 心跳回复 | ✅ 已实现 |
| `error` | `{ "message" }` | 错误 | ✅ 已实现 |
| `assistant_audio` | `{ "url": "/uploads/audio/xxx.mp3", "text": "回复文本" }` | 回复语音（回复完成后自动 TTS 合成，url 为相对路径，前端拼接后端地址） | ✅ 已实现 |
| `proactive_message` | `{ "text" }` | 主动推送（情绪超阈值，LLM 生成 + 存入记忆） | ✅ 已实现 |
| `emotion_update` | `{ "state", "description", "top" }` | 情绪状态更新（回复结束时推送） | ✅ 已实现 |
| `mcp_capabilities` | `{ "servers": [...] }` | MCP 能力更新（任意端操作 MCP 后广播，前端实时同步） | ✅ 已实现 |
| `settings_update_ack` | `{ "section", "ok", "message" }` | 配置修改确认 | ✅ 已实现 |
| `mcp_installed_list_ack` | `{ "servers": [...] }` | 已安装列表确认 | ✅ 已实现 |
| `mcp_toggle_ack` | `{ "id", "ok", "enabled" }` | 启用/禁用确认 | ✅ 已实现 |
| `mcp_tool_toggle_ack` | `{ "server", "tool", "enabled", "ok" }` | 工具开关确认 | ✅ 已实现 |
| `mcp_market_list_ack` | `{ "items": [...] }` | 市场列表确认 | ✅ 已实现 |
| `mcp_install_ack` | `{ "id", "ok", "message" }` | 安装确认 | ✅ 已实现 |
| `mcp_uninstall_ack` | `{ "id", "ok", "message" }` | 卸载确认 | ✅ 已实现 |
| `mcp_configure_ack` | `{ "id", "ok", "message" }` | 配置修改确认 | ✅ 已实现 |
| `upload_complete_ack` | `{ "ok", "path", "file_name", "size" }` | 分块上传完成（path 为 /uploads/files/... 相对路径） | ✅ 已实现 |

### 1.3 示例：完整对话流程

```text
前端 → 后端:  { "type": "handshake", "payload": { "session_id": "my-device-01" } }
后端 → 前端:  { "type": "handshake_ack", "payload": { "name": "Alice", "llm": "deepseek-chat", "version": "0.1.0", "session_id": "my-device-01" } }

前端 → 后端:  { "type": "user_message", "payload": { "text": "今天好累", "session_id": "my-device-01" } }
后端 → 前端:  { "type": "assistant_chunk", "payload": { "content": "怎么" } }
后端 → 前端:  { "type": "assistant_chunk", "payload": { "content": "了？" } }
后端 → 前端:  { "type": "assistant_chunk", "payload": { "content": "", "done": true } }

前端 → 后端:  { "type": "ping" }
后端 → 前端:  { "type": "pong", "payload": { "time": 1785655350 } }
```

### 1.4 会话说明

- `session_id` 保留在协议层（多端可各自持有），但 Alice 的记忆/情绪/历史是**全局统一**的——私人部署，任何端看到的都是同一份
- 不携带 `session_id` 同样正常使用（归入默认会话）
- 后端会发送心跳 Ping（约 54s），客户端无需处理，但应响应 Pong 帧或保持活跃

---

## 二、HTTP API（`/api/v1`）

| 端点 | 方法 | 说明 | 状态 |
|------|------|------|------|
| `/api/v1/health` | GET | 服务健康检查（含 LLM / Embedding / 记忆数） | ✅ 已实现 |
| `/api/v1/settings` | GET | 当前可调设置（LLM / 情绪 / 记忆容量；api_key 只返回是否已配置） | ✅ 已实现 |
| `/api/v1/memory/block` | GET | 获取 Memory Block 内容（只读） | ✅ 已实现 |
| `/api/v1/memory/block/:id` | GET | 获取单条记忆详情 | ✅ 已实现 |
| `/api/v1/memory/search` | POST | 搜索 RAG 历史（body: `{"query": "..."}`） | ✅ 已实现 |
| `/api/v1/memory/export` | GET | 导出所有记忆（JSON 下载） | ✅ 已实现 |
| `/api/v1/mcp/status` | GET | 已安装 MCP Server 状态（含工具级开关） | ✅ 已实现 |
| `/api/v1/mcp/market` | GET | MCP 市场可安装项列表 | ✅ 已实现 |
| `/api/v1/emotion/events` | GET | 情绪显著事件记录（`?limit=` 可选） | ✅ 已实现 |
| `/api/v1/emotion/proactive` | GET / POST | 主动推送开关（GET 查询 / POST `{"enabled":bool}` 切换，持久化） | ✅ 已实现 |
| `/api/v1/audio/stt` | POST | 语音转文字（multipart `file` 上传音频 → `{"text": "..."}`） | ✅ 已实现 |
| `/api/v1/history` | GET | 当天聊天记录（`?date=YYYY-MM-DD` 可选） | ✅ 已实现 |
| `/api/v1/history/dates` | GET | 已归档日期列表 | ✅ 已实现 |
| `/uploads/...` | GET | 静态文件服务（TTS 音频 / 用户上传文件） | ✅ 已实现 |

### `GET /api/v1/health`

```json
{ "status": "ok", "llm": "deepseek-chat (mock)", "embedding": "BAAI/bge-m3 (hash)", "memory": 42 }
```

### 主动推送（`proactive_message`）

情绪超阈值时触发，机制：
- 话术由 **LLM 根据当前情绪实时生成**（非静态模板）
- 生成的主动消息会**存入 RAG + Memory Block**（Alice 记得自己主动说过什么）
- 用户超过 `silent_after_minutes`（默认 30 分钟）无互动 → 触发失落/焦虑上升 + 主动问候
- `hours` 限制推送时段（默认 8–23 点），避免深夜打扰
- 推送间有 `cooldown_seconds` 冷却，防止打扰

情绪显著事件（变化量 > 0.1）记录到 Redis，可通过 `GET /api/v1/emotion/events` 查询历史轨迹。

---

## 三、语音能力（TTS / STT）

语音由两个 **MCP 内部 Server** 提供（`mcp.yaml` 中 `internal: true`，工具不暴露给 LLM——音频数据不进 LLM 上下文，仅后端直接调用）。

### 语音合成（TTS）

- 回复完成后后端自动调用 `tts__speak` 合成语音，保存到 `uploads/audio/`，推送 `assistant_audio`（相对路径，前端拼接后端地址播放）
- 开关：`kernel.yaml` → `audio.tts_enabled`（默认 true）
- 引擎优先级（环境变量，密钥不进仓库）：
  1. `ALICE_TTS_API_KEY` → OpenAI 兼容 `/audio/speech`（默认硅基流动 `FunAudioLLM/CosyVoice2-0.5B`，`ALICE_TTS_BASE_URL` / `ALICE_TTS_MODEL` / `ALICE_TTS_VOICE` 可覆盖）
  2. 未配置 → **Edge TTS 免费接口**（无需 key，需访问微软网络；`ALICE_TTS_EDGE_VOICE` 指定音色，默认自动尝试 zh-CN 可用音色）

### 语音识别（STT）

- 前端录音（MediaRecorder）→ `POST /api/v1/audio/stt`（multipart `file`）→ `{"text": "..."}`
- 开关：`kernel.yaml` → `audio.stt_enabled`（默认 true）
- 引擎优先级：
  1. `ALICE_STT_API_KEY` → OpenAI 兼容 `/audio/transcriptions`（默认硅基流动 `iic/SenseVoiceSmall`，`ALICE_STT_BASE_URL` / `ALICE_STT_MODEL` 可覆盖）
  2. 未配置 → 本地 whisper.cpp CLI（`ALICE_WHISPER_BIN` 默认 `whisper-cli`，`ALICE_WHISPER_MODEL` 默认 `models/ggml-base.bin`）

### 文件上传（`upload_chunk`）

分块上传协议（WebSocket）：

```
前端 → 后端:  { "type": "upload_start", "payload": { "upload_id", "file_name", "total_chunks" } }
前端 → 后端:  [Binary Frame] × total_chunks（顺序发送，WebSocket 保序）
前端 → 后端:  { "type": "upload_end", "payload": { "upload_id" } }
后端 → 前端:  { "type": "upload_complete_ack", "payload": { "ok", "path", "file_name", "size" } }
```

- 文件保存到 `uploads/files/`（文件名清洗防路径穿越），大小上限 `audio.max_upload_mb`（默认 20MB）
- 上传后可把 `path` 附在消息中（如 `[图片:/uploads/files/xxx.png]`），Alice 可用视觉 MCP 查看图片

---

## 四、错误格式

所有错误通过 WebSocket `error` 消息或 HTTP 非 2xx 状态码返回：

```json
{ "message": "错误描述" }
```

---

## 五、版本约定

- API 前缀 `/api/v1` 稳定不变，破坏性变更将升级为 `/api/v2`
- 后端 `version` 字段（`0.1.0`）在 `handshake_ack` 中返回，客户端可据此做兼容判断
