# Alice

拟人化 AI 陪伴系统。一个**有情绪、有记忆、有自我认知**的 AI——用户不需要迁就她，不需要记住她的设定，不需要为她打圆场。

- **不撒谎**：始终知道自己是个 AI，不假装有身体、有真实情感
- **不迁就**：用户说错话、说穿帮话，Alice 自己兜得住
- **有温度**：有情绪、有记忆、会主动、会想人
- **低心智负担**：用户不需要记设定、不需要打圆场、不需要维护上下文

## 架构

```
┌─────────────┐   WebSocket / HTTP    ┌──────────────────────┐   MCP    ┌──────────────┐
│  Frontend   │ ────────────────────► │  Backend (Go Kernel) │ ───────► │  MCP Server  │
│ Astro+Vue3  │   （解耦，多端适用）   │  System + Memory +   │          │  TTS/STT/...  │
│  纯壳子     │                       │  Emotion + RAG + LLM │          └──────────────┘
└─────────────┘                       └──────────┬───────────┘
                                                 │ YAML
                                        ┌────────▼─────────┐
                                        │      Config      │
                                        └──────────────────┘
```

前后端**完全解耦**：前端只知道一个后端地址，HTTP 与 WebSocket 均由它推导；后端是纯 API 服务，任意多端（Web / 移动 / 桌面）可接入。详见 [API 文档](docs/api.md)。

## 快速开始

```bash
# 后端（Go 1.22+）
cd backend
go run .                  # 监听 :8081（端口在 config/main.yaml）

# 前端（Node 22+）
cd frontend
npm install
npm run dev               # http://localhost:4321
```

打开 `http://localhost:4321` 即可聊天。

**LLM 配置**（`config/kernel.yaml`）：

```yaml
llm:
  provider: "deepseek"            # deepseek / openai / siliconflow / mock
  base_url: "https://api.deepseek.com/v1"
  api_key: ""                     # 留空则自动进入 mock 模式（模拟回复，开发用）
  model: "deepseek-chat"
```

支持任意 OpenAI 兼容 API（DeepSeek / 硅基流动 / OpenAI / Ollama 等），只需改 `base_url` + `api_key` + `model`。硅基流动示例：

```yaml
llm:
  provider: "siliconflow"
  base_url: "https://api.siliconflow.cn/v1"
  api_key: "sk-xxx"
  model: "deepseek-ai/DeepSeek-V3"
```

## 前端配置后端地址

优先级：**运行时设置（localStorage）> `frontend/public/config.json` > 自动推导**。

- 页面右上角「设置」→ 输入任意后端地址 → 保存并重连，无需重新构建
- 部署后可编辑 `public/config.json` 的 `backendUrl` 字段
- 默认推导：开发模式 → 本机 `:8081`；同源部署 → 当前站点地址

## 配置

所有可调参数集中在 [`config/`](config/)，不改代码：

```
config/
├── main.yaml              # 服务端口、日志
├── kernel.yaml            # LLM、上下文
├── emotion.yaml           # 情绪引擎（阶段三）
├── emotion_events.yaml    # 事件→情绪映射（阶段三）
├── memory_rag.yaml        # RAG（阶段二）
├── memory_block.yaml      # Memory Block（阶段二）
├── mcp.yaml               # MCP（阶段四）
└── prompts/
    ├── system_prompt.txt  # Alice 人格底座
    └── emotion_templates.yaml
```

## 路线图

| 阶段 | 内容 | 状态 |
|------|------|------|
| 阶段一 | 对话闭环：Config + LLM（流式/Function Calling 预留/Mock）+ WebSocket + 前端对话页 | ✅ 完成 |
| 阶段一+ | 前后端解耦：运行时后端地址、CORS、多会话（session_id） | ✅ 完成 |
| 阶段二 | 记忆系统：Memory Block（去重/容量）+ RAG（Embedding + Redis 检索 + 落库） | ✅ 完成 |
| 阶段三 | 情绪引擎：高维向量、事件驱动、时间衰减、主动推送、持久化 | ✅ 完成 |
| 阶段四 | MCP 层：MCP Manager + 注册表 + TTS/STT/Vision/Search | ⏳ 规划中 |
| 阶段五 | 前端补全：设置面板、MCP 市场、情绪可视化、主题切换 | ⏳ 规划中 |

## 目录结构

```
alice/
├── backend/            # Go 后端（纯 API）
│   ├── kernel/         # 对话核心 + LLM Client
│   ├── server/         # WebSocket / HTTP / CORS
│   └── config/         # YAML 加载
├── frontend/           # Astro + Vue3 + Pinia + Tailwind
│   ├── src/components/ # chat / settings / (emotion, memory, mcp 待建)
│   ├── src/services/   # WebSocket、后端地址解析
│   └── src/stores/     # Pinia
├── config/             # 配置文件 + prompts
├── docs/               # 文档（API 等）
└── registry/           # MCP 注册表缓存（阶段四）
```

## 文档

- [设计文档](desgin.md) — 总体设计
- [API 文档](docs/api.md) — WebSocket / HTTP 协议
