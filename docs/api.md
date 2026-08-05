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
| `upload_chunk` | Binary Frame | 上传文件分块 | 🚧 规划中 |
| `settings_update` | `{...}` | 修改配置 | 🚧 规划中 |
| `mcp_market_list` | — | 获取 MCP 市场列表 | 🚧 规划中 |
| `mcp_install` | `{...}` | 安装 MCP | 🚧 规划中 |
| `mcp_uninstall` | `{...}` | 卸载 MCP | 🚧 规划中 |
| `mcp_toggle` | `{...}` | 启用/禁用 MCP | 🚧 规划中 |
| `mcp_configure` | `{...}` | 修改 MCP 配置 | 🚧 规划中 |
| `mcp_installed_list` | — | 获取已安装 MCP 列表 | ✅ 已实现 |
| `mcp_toggle` | `{ "id": "serverId", "enabled": true/false }` | 启用/禁用 MCP | ✅ 已实现 |
| `mcp_tool_toggle` | `{ "server", "tool", "enabled" }` | 工具级启用/禁用（持久化） | ✅ 已实现 |
| `mcp_market_list` | — | 获取 MCP 市场列表 | ✅ 已实现 |
| `mcp_install` | `{ "id" }` | 安装 MCP（写入 mcp.yaml + 热重载） | ✅ 已实现 |
| `mcp_uninstall` | `{ "id" }` | 卸载 MCP | ✅ 已实现 |
| `mcp_configure` | `{...}` | 修改 MCP 配置 | 🚧 规划中 |

### 1.2 后端 → 前端

| type | payload | 说明 | 状态 |
|------|---------|------|------|
| `handshake_ack` | `{ "name", "llm", "version", "serverTime", "session_id" }` | 握手确认 | ✅ 已实现 |
| `assistant_chunk` | `{ "content": "增量文本", "done": true/false }` | 流式回复（逐字推送，`done:true` 结束） | ✅ 已实现 |
| `pong` | `{ "time" }` | 心跳回复 | ✅ 已实现 |
| `error` | `{ "message" }` | 错误 | ✅ 已实现 |
| `assistant_audio` | `{...}` | 回复音频（TTS） | 🚧 规划中 |
| `proactive_message` | `{ "text" }` | 主动推送（情绪超阈值触发） | ✅ 已实现 |
| `emotion_update` | `{ "state", "description", "top" }` | 情绪状态更新（回复结束时推送） | ✅ 已实现 |
| `mcp_capabilities` | `{...}` | MCP 能力更新 | 🚧 规划中 |
| `settings_update_ack` | `{...}` | 配置修改确认 | 🚧 规划中 |
| `mcp_installed_list_ack` | `{ "servers": [...] }` | 已安装列表确认 | ✅ 已实现 |
| `mcp_toggle_ack` | `{ "id", "ok", "enabled" }` | 启用/禁用确认 | ✅ 已实现 |
| `mcp_*_ack` | `{...}` | MCP 操作确认 | ✅ 已实现 |

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

- 客户端可在握手或每次 `user_message` 中携带 `session_id` 绑定会话
- 不携带则归入默认会话 `"default"`
- 不同 `session_id` 的上下文完全隔离（多端互不干扰）
- 后端会发送心跳 Ping（约 54s），客户端无需处理，但应响应 Pong 帧或保持活跃

---

## 二、HTTP API（`/api/v1`）

| 端点 | 方法 | 说明 | 状态 |
|------|------|------|------|
| `/api/v1/health` | GET | 服务健康检查（含 LLM / Embedding / 记忆数） | ✅ 已实现 |
| `/api/v1/memory/block` | GET | 获取 Memory Block 内容（只读） | ✅ 已实现 |
| `/api/v1/memory/block/:id` | GET | 获取单条记忆详情 | ✅ 已实现 |
| `/api/v1/memory/search` | POST | 搜索 RAG 历史（body: `{"query": "..."}`） | ✅ 已实现 |
| `/api/v1/memory/export` | GET | 导出所有记忆（JSON 下载） | ✅ 已实现 |
| `/api/v1/mcp/status` | GET | 已安装 MCP Server 状态（含工具级开关） | ✅ 已实现 |
| `/api/v1/mcp/market` | GET | MCP 市场可安装项列表 | ✅ 已实现 |
| `/api/v1/emotion/events` | GET | 情绪显著事件记录（`?limit=` 可选） | ✅ 已实现 |
| `/api/v1/history` | GET | 当天聊天记录（`?date=YYYY-MM-DD` 可选） | ✅ 已实现 |
| `/api/v1/history/dates` | GET | 已归档日期列表 | ✅ 已实现 |

### `GET /api/v1/health`

```json
{ "status": "ok", "llm": "deepseek-chat (mock)" }
```

---

## 三、错误格式

所有错误通过 WebSocket `error` 消息或 HTTP 非 2xx 状态码返回：

```json
{ "message": "错误描述" }
```

---

## 四、版本约定

- API 前缀 `/api/v1` 稳定不变，破坏性变更将升级为 `/api/v2`
- 后端 `version` 字段（`0.1.0`）在 `handshake_ack` 中返回，客户端可据此做兼容判断
