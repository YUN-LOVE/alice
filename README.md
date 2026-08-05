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

# 内置 MCP Server（需先编译一次）
cd mcp-server && go build -o alice-local-tools . && cd ..
cd mcp-vision && go build -o alice-vision-tools . && cd ..   # 视觉（需视觉模型 key）

# 前端（Node 22+，包管理器用 pnpm）
cd frontend
pnpm install
pnpm dev                  # http://localhost:4321（监听 0.0.0.0，局域网可访问）
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

所有可调参数集中在 [`config/`](config/)，不改代码。**配置文件支持热重载**：修改后约 1 秒内自动生效（LLM 参数、System Prompt、情绪引擎、记忆容量等），无需重启后端。

MCP Server 在 `config/mcp.yaml` 的 `servers` 列表注册，后端启动时自动拉起。支持两种接入方式：

- **stdio**：本地二进制或 `npx` 启动（默认）
- **http**：远程 MCP Server 通过 URL 接入（第三方 MCP 常见），配置 `transport: "http"` + `url` + 可选 `headers`（如 `Authorization`）

**视觉能力**（`mcp-vision/`）：工具 `describe_image`（本地图片）/ `describe_image_url`（网络图片），由视觉模型描述。配置环境变量（密钥不进仓库）：

```bash
export ALICE_VISION_API_KEY=sk-xxx          # 硅基流动等 OpenAI 兼容视觉模型 key
export ALICE_VISION_MODEL=Qwen/Qwen2.5-VL-72B-Instruct   # 可选，默认如上
```

## 历史聊天记录

每轮对话按时间戳存入 RAG 并打**日期 tag**。前端页面加载时显示**当天的聊天记录**；每天零点自动整理归档（前一天对话归入长期记忆）。没有会话概念——Alice 只有全局一份记忆，私人部署，任何端看到的都是同一份。

## 情绪与主动推送

情绪引擎完整实现设计文档的三大机制：

- **事件驱动**：`emotion_events.yaml` 关键词 → 事件 → 情绪向量变化
- **时间演化**：指数衰减趋近 baseline + **关系矩阵**（`emotion.yaml` 的 `relations`：漂移/拮抗，如焦虑抑制开心）
- **主动推送**：情绪超阈值 → **LLM 根据当前情绪生成自然的关心话**（非静态模板）→ 存入 RAG + Block（Alice 记得自己主动说过什么）→ 广播到所有连接
  - `silent_after_minutes`：用户超过该时长无互动，触发失落/焦虑上升 + 主动问候
  - `hours`：允许推送的时段（默认 8–23 点，不深夜打扰）
  - 显著情绪事件记录到 Redis，`GET /api/v1/emotion/events` 可查情绪历史轨迹

## 前端

- 对话消息按 **Markdown 渲染**（标题/列表/代码块/表格/链接），DOMPurify 防 XSS
- 侧边面板：记忆查看 / MCP 管理（工具级开关）/ 情绪可视化
- 主题切换：暗色 / 亮色
- 历史记录：页面刷新自动恢复当天聊天

```
config/
├── main.yaml              # 服务端口、日志
├── kernel.yaml            # LLM、上下文
├── emotion.yaml           # 情绪引擎（维度/关系矩阵/衰减/主动推送参数）
├── emotion_events.yaml    # 事件→情绪映射（关键词触发）
├── memory_rag.yaml        # RAG（阶段二）
├── memory_block.yaml      # Memory Block（阶段二）
├── mcp.yaml               # MCP（servers / 注册表 / 传输方式）
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
| 阶段四 | MCP 层：stdio 协议客户端、生命周期管理、Function Calling 循环、内置工具 | ✅ 完成 |
| 阶段四+ | Vision：视觉 MCP Server（describe_image，帮 Alice "看"图片） | ✅ 完成 |
| 阶段四++ | MCP 市场：注册表 + 安装/卸载 + 工具级开关（可单独决定用哪个工具） | ✅ 完成 |
| 阶段五 | 前端补全：侧边面板（记忆查看 / MCP 管理 / 情绪可视化）、主题切换、Markdown 渲染 | ✅ 完成 |
| Kernel+ | 主动推送增强（LLM 生成 + 存记忆 + silent 触发 + 时段）、情绪记忆、关系矩阵 | ✅ 完成 |

## 目录结构

```
alice/
├── backend/            # Go 后端（纯 API）
│   ├── kernel/         # 对话核心 + LLM Client + 主动推送
│   ├── emotion/        # 情绪引擎（向量/关系矩阵/事件记录）
│   ├── memory/         # 记忆系统（RAG + Memory Block）
│   ├── mcp/            # MCP 管理（stdio/HTTP 客户端、市场、注册表）
│   ├── server/         # WebSocket / HTTP / CORS
│   └── config/         # YAML 加载 + 热重载
├── frontend/           # Astro + Vue3 + Pinia + Tailwind
│   ├── src/components/ # chat / settings / panel / memory / mcp / emotion
│   ├── src/services/   # WebSocket、后端地址解析、API、Markdown 渲染
│   └── src/stores/     # Pinia（chat / mcp / memory）
├── config/             # 配置文件 + prompts
├── docs/               # 文档（API 等）
├── mcp-server/         # 内置 MCP：计算器/时间/回显
├── mcp-vision/         # 视觉 MCP Server
└── registry/           # MCP 市场注册表
```

## 文档

- [设计文档](desgin.md) — 总体设计
- [API 文档](docs/api.md) — WebSocket / HTTP 协议
