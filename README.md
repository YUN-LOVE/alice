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

### 0. 依赖

- **Redis**（必需：记忆 / 情绪 / 历史 / MCP 开关都依赖）

```bash
# 安装（任选其一）
sudo pacman -S redis        # Arch
sudo apt install redis-server  # Debian/Ubuntu

# 启动
redis-server --daemonize yes --port 6379
redis-cli ping               # 应返回 PONG
```

- **Node 22+**（前端，包管理器用 pnpm）、**Go 1.22+**（后端）

### 1. 构建内置 MCP Server + 后端

```bash
# 在项目根目录
make mcp        # 编译 mcp-server/alice-local-tools、mcp-vision/alice-vision-tools、
                # mcp-tts/alice-tts（语音合成）、mcp-stt/alice-stt（语音识别）
make backend    # 编译 backend/alice-server
```

### 2. 启动

```bash
# 后端
cd backend && go run .     # 或 ./alice-server（make backend 产物），监听 :8081

# 前端
cd frontend && pnpm dev    # http://localhost:4321（监听 0.0.0.0，局域网可访问）
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

## 语音能力（TTS / STT）

Alice 现在**会说话、会听**（设计文档 P1 语音能力）：

- **语音输出**：回复完成后自动合成语音（`assistant_audio`），前端底部出现播放条（播放/暂停/进度）；`kernel.yaml` → `audio.tts_enabled` 开关
- **语音输入**：输入框 🎤 录音 → 转文字填入输入框；`audio.stt_enabled` 开关
- **发送图片**：输入框 🖼️ 选择图片 → 分块上传（`upload_chunk`）→ 消息带图，Alice 用视觉 MCP 查看

TTS / STT 由两个 **MCP 内部 Server** 提供（`mcp.yaml` 中 `internal: true`，音频数据不进 LLM 上下文）。引擎优先级（密钥不进仓库，环境变量）：

```bash
# TTS：配 key 走 OpenAI 兼容 API（默认硅基流动 CosyVoice2-0.5B）；不配则用 Edge TTS 免费接口
export ALICE_TTS_API_KEY=sk-xxx
# 可选（API 模式）：export ALICE_TTS_BASE_URL / ALICE_TTS_MODEL / ALICE_TTS_VOICE

# Edge TTS 模式可配置项（未配 key 时生效，缺省即用，工具调用参数可覆盖）：
export ALICE_TTS_EDGE_VOICE="zh-CN-XiaoyiNeural"   # 音色（留空自动尝试候选列表）
export ALICE_TTS_EDGE_RATE="+10%"                  # 语速：+10% 快一成 / -20% 慢两成
export ALICE_TTS_EDGE_PITCH="+0Hz"                 # 音高：+5Hz 更尖 / -3Hz 更低沉
export ALICE_TTS_EDGE_VOLUME="+0%"                 # 音量：+20% 更大 / -50% 更小

# STT：配 key 走 OpenAI 兼容 API（默认硅基流动 SenseVoiceSmall）；不配则尝试本地 whisper.cpp
export ALICE_STT_API_KEY=sk-xxx
# 可选：export ALICE_STT_BASE_URL / ALICE_STT_MODEL / ALICE_WHISPER_BIN / ALICE_WHISPER_MODEL
```

**语音参数说明**：`speak` 工具的 `voice/rate/pitch/volume` 参数优先级高于环境变量（方便前端或脚本按句控制音色语气）；只配环境变量则全局生效。常用中文音色示例：`zh-CN-XiaoyiNeural`（女声）、`zh-CN-YunxiNeural`（男声）、`en-US-EmmaMultilingualNeural`（官方默认多语言女声）。

## 运行时设置（设置面板）

前端「设置 → 对话」可修改 **LLM（provider/base_url/api_key/model/temperature/max_tokens）**、**情绪引擎（阈值/冷却/衰减率/静默触发等）**、**Memory Block 容量**，保存后写回 YAML 并热重载生效（保留注释与键顺序）。MCP 面板可修改 Server 的 `args/env/url` 配置。

## 历史聊天记录

每轮对话按时间戳存入 RAG 并打**日期 tag**。前端页面加载时显示**当天的聊天记录**；每天零点自动整理归档（前一天对话归入长期记忆）。没有会话概念——Alice 只有全局一份记忆，私人部署，任何端看到的都是同一份。

## 情绪与主动推送

情绪引擎完整实现设计文档的三大机制：

- **事件驱动**：`emotion_events.yaml` 关键词 → 事件 → 情绪向量变化
- **时间演化**：指数衰减趋近 baseline + **关系矩阵**（`emotion.yaml` 的 `relations`：漂移/拮抗，如焦虑抑制开心）
- **主动推送**：情绪超阈值 → **LLM 根据当前情绪生成自然的关心话**（提示词模板在 `config/prompts/proactive_prompt.txt`）→ 存入 RAG + Block（Alice 记得自己主动说过什么）→ 广播到所有连接
  - `silent_after_minutes`：用户超过该时长无互动，触发失落/焦虑上升 + 主动问候
  - `skip_if_active_minutes`：用户最近互动过则跳过（聊天中不打扰）
  - `hours`：允许推送的时段（默认 8–23 点，不深夜打扰）
  - **mock 模式（未配 key）自动跳过主动推送**
  - 显著情绪事件记录到 Redis，`GET /api/v1/emotion/events` 可查情绪历史轨迹

## 前端

- **Material You 动态主题**：Monet 取色引擎生成 M3 完整色阶（5 层 surface 渐变）
  - **9 种取色算法**：经典 / 表现 / 活力 / 单色 / 中性 / 忠实 / 彩虹 / 果昔 / 内容（`@material/material-color-utilities`）
  - **壁纸取色**：设置面板「外观」→「从壁纸取色」上传图片即自动生成主题（优先提取高饱和主色，自动提亮提彩兜底）
  - 预设色板 + 亮/暗模式，持久化到本地
- 对话消息按 **Markdown 渲染**（标题/列表/代码块/表格/链接），DOMPurify 防 XSS
- 侧边面板：记忆查看 / MCP 管理（工具级开关）/ 情绪可视化
- 历史记录：页面刷新自动恢复当天聊天

```
config/
├── main.yaml              # 服务端口、日志
├── kernel.yaml            # LLM、上下文、语音（audio）
├── emotion.yaml           # 情绪引擎（维度/关系矩阵/衰减/主动推送参数）
├── emotion_events.yaml    # 事件→情绪映射（关键词触发）
├── memory_rag.yaml        # RAG（阶段二）
├── memory_block.yaml      # Memory Block（阶段二）
├── mcp.yaml               # MCP（servers / 注册表 / 传输方式 / internal）
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
| 阶段六 | 语音能力：TTS/STT MCP Server（API + Edge/whisper 兜底）、assistant_audio、前端录音与播放控制 | ✅ 完成 |
| 阶段六+ | 运行时设置：settings_update 写回 YAML + 热重载、设置面板（LLM/情绪/记忆）、mcp_configure、mcp_capabilities、upload_chunk 文件上传 | ✅ 完成 |

## 目录结构

```
alice/
├── backend/            # Go 后端（纯 API）
│   ├── kernel/         # 对话核心 + LLM Client + 主动推送 + 语音/设置
│   ├── emotion/        # 情绪引擎（向量/关系矩阵/事件记录）
│   ├── memory/         # 记忆系统（RAG + Memory Block）
│   ├── mcp/            # MCP 管理（stdio/HTTP 客户端、市场、注册表）
│   ├── server/         # WebSocket / HTTP / CORS / 分块上传
│   └── config/         # YAML 加载 + 热重载 + 设置写回
├── frontend/           # Astro + Vue3 + Pinia + Tailwind
│   ├── src/components/ # chat / settings / panel / memory / mcp / emotion
│   ├── src/services/   # WebSocket、后端地址解析、API、Markdown、语音、上传
│   └── src/stores/     # Pinia（chat / mcp / memory）
├── config/             # 配置文件 + prompts
├── docs/               # 文档（API 等）
├── mcp-server/         # 内置 MCP：计算器/时间/回显
├── mcp-vision/         # 视觉 MCP Server
├── mcp-tts/            # 语音合成 MCP Server（API + Edge TTS 兜底）
├── mcp-stt/            # 语音识别 MCP Server（API + whisper 兜底）
└── registry/           # MCP 市场注册表
```

## 文档

- [设计文档](desgin.md) — 总体设计
- [API 文档](docs/api.md) — WebSocket / HTTP 协议
