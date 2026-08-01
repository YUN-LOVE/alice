Alice — 项目总体设计文档

---

一、项目概述

Alice 是一个拟人化AI陪伴系统。她是一个有情绪、有记忆、有自我认知的AI，用户不需要迁就她，不需要记住她的设定，不需要为她打圆场。

核心哲学：

· 不撒谎：Alice始终知道自己是个AI，不假装有身体、有真实情感
· 不迁就：用户说错话、说穿帮话，Alice自己兜得住
· 有温度：她有情绪、有记忆、会主动、会想人
· 低心智负担：用户不需要记设定、不需要打圆场、不需要维护上下文

二、总体架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Alice 系统                                       │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      前端（Vue/TypeScript）                         │   │
│  │  职责：UI展示 + 用户交互                                           │   │
│  │  通信：WebSocket（实时）+ HTTP（设置/记忆查看）                    │   │
│  │  功能：对话界面 / 设置面板 / MCP市场 / 情绪可视化                  │   │
│  └──────────────────────────────┬──────────────────────────────────────┘   │
│                                 │ WebSocket / HTTP                         │
│                                 ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    后端主进程（Go/Kernel）                         │   │
│  │  职责：思考、决策、生成回复                                        │   │
│  │                                                                     │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐             │   │
│  │  │ System       │  │ Memory Block │  │   Emotion    │             │   │
│  │  │ Prompt       │  │ 短期工作记忆  │  │   情绪引擎   │             │   │
│  │  │ （你是谁）   │  │ （增量注入）  │  │  （高维向量）│             │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘             │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐             │   │
│  │  │     RAG      │  │   LLM Client │  │ MCP Client   │             │   │
│  │  │  长期记忆    │  │  调用大模型  │  │ 调用外设工具 │             │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘             │   │
│  └──────────────────────────────┬──────────────────────────────────────┘   │
│                                 │ MCP协议（stdio / HTTP）                   │
│                                 ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                     MCP Server（Go）                               │   │
│  │  职责：把外部API标准化，暴露工具给Kernel                            │   │
│  │                                                                     │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐          │   │
│  │  │   TTS    │  │   STT    │  │  Vision  │  │  Search  │          │   │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘          │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         Config（YAML）                              │   │
│  │  所有模块配置，统一管理，不改代码                                    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

三、模块详述

1. Kernel（大脑）

职责：思考、决策、生成回复。

子模块 职责 关键特性
System Prompt Alice的身份底座 固定不变，定义"你是谁"
Memory Block 短期工作记忆 存RAG查询原文，增量注入，默认无限
Emotion Engine 情绪状态 高维向量，事件驱动，时间演化
RAG 长期记忆 云端Embedding+本地Redis，每轮查询+存储
LLM Client 生成回复 OpenAI兼容API，Function Calling集成MCP工具

工作流程：

```
用户输入（WebSocket）
    │
        ▼
        [1] Emotion Engine：处理事件，更新情绪向量
            │
                ▼
                [2] RAG：查询相关历史记忆
                    │
                        ▼
                        [3] Memory Block：增量注入新记忆（去重，不重复）
                            │
                                ▼
                                [4] MCP Client：获取可用工具列表（已安装且启用的MCP）
                                    │
                                        ▼
                                        [5] 构建上下文：System + Emotion描述 + Memory Block + 用户输入 + 可用工具
                                            │
                                                ▼
                                                [6] LLM：生成回复（如需外设则通过MCP Client调用工具）
                                                    │
                                                        ▼
                                                        [7] 存储：本轮对话存入RAG（全量）
                                                            │
                                                                ▼
                                                                [8] 返回回复（WebSocket流式）
                                                                ```

                                                                2. Emotion Engine（情绪引擎）

                                                                设计：高维向量空间。

                                                                数据结构：

                                                                · 情绪向量：{"开心": 0.6, "失落": 0.2, "温柔": 0.5, ...}
                                                                · 维度关系矩阵：定义情绪之间互相转化/抑制/拮抗
                                                                · 情绪记忆：记录显著情绪事件

                                                                核心机制：

                                                                · 事件驱动：用户行为 → 情绪向量变化
                                                                · 时间演化：情绪自然衰减、漂移、转化
                                                                · 阈值触发：情绪超阈值触发主动输出

                                                                对外接口：

                                                                · process_event(event) → 更新情绪
                                                                · tick(dt) → 时间步进
                                                                · get_emotion_summary() → 返回情绪描述文本和风格提示

                                                                3. Memory（记忆系统）

                                                                分层设计：

                                                                层级 名称 存储 容量 更新机制
                                                                L1 Memory Block 内存 默认无限（可配置） 增量注入RAG结果
                                                                L2 RAG（向量库） Redis 永久 每轮自动存储

                                                                Memory Block（短期工作记忆）：

                                                                · 存储RAG查询结果的原文（不压缩、不摘要）
                                                                · 增量注入：只加没有的，不重复
                                                                · 直接注入上下文，LLM自然引用
                                                                · 可配置最大条数，0=无限
                                                                · 前端只读，不能删除

                                                                RAG（长期记忆）：

                                                                · 全量存储：每轮对话原文 + 元数据（时间戳、情绪快照）
                                                                · 云端Embedding生成向量（固定模型，不切换）
                                                                · 本地Redis向量检索（低延迟）
                                                                · 每轮自动查询，结果通过Memory Block注入

                                                                4. MCP Server（外设适配层）

                                                                职责：屏蔽外部API差异，给Kernel提供统一接口。

                                                                接入方式：MCP协议（Go官方SDK），Kernel通过MCP Client调用。

                                                                可暴露的工具：

                                                                外设 功能 可选供应商
                                                                TTS 文字→语音 Edge / Azure / 本地
                                                                STT 语音→文字 Whisper / 讯飞
                                                                Vision 图像识别 GPT-4V / Claude / 本地
                                                                Search 联网搜索 Google / Bing / 百度

                                                                部署方式：

                                                                · stdio：子进程模式，Kernel启动时自动拉起（推荐，本地部署）
                                                                · HTTP/SSE：独立服务，分布式部署

                                                                Embedding特殊处理：不走MCP，内嵌在RAG里，作为RAG的底层依赖。

                                                                5. MCP Manager（MCP生命周期管理，后端Go）

                                                                职责：管理所有MCP Server的安装、启动、启用/禁用、卸载。

                                                                功能 说明
                                                                安装 从注册表下载MCP二进制，启动子进程，注册工具
                                                                启动 启动MCP子进程，建立MCP连接，获取工具列表
                                                                启用/禁用 启用时连接MCP并注册工具；禁用时断开连接，移除工具
                                                                卸载 停止进程，删除二进制，从列表中移除
                                                                配置更新 修改MCP配置，重启生效

                                                                6. Config（配置层）

                                                                设计原则：所有可调参数抽离到配置文件，不改代码。

                                                                结构：

                                                                ```
                                                                config/
                                                                ├── main.yaml              # 主入口
                                                                ├── kernel.yaml            # Kernel配置（LLM、上下文）
                                                                ├── emotion.yaml           # 情绪引擎配置（维度、关系、衰减）
                                                                ├── emotion_events.yaml    # 事件→情绪向量映射
                                                                ├── memory_rag.yaml        # RAG配置（Embedding、Redis、检索）
                                                                ├── memory_block.yaml      # Memory Block配置（容量、注入）
                                                                ├── mcp.yaml               # MCP配置（注册表地址、MCP列表）
                                                                └── prompts/
                                                                    ├── system_prompt.txt      # 固定人格
                                                                        └── emotion_templates.yaml # 情绪→文本映射
                                                                        ```

                                                                        注册表（MCP市场数据源）：可配置为本地文件或远程URL。

                                                                        四、前端设计

                                                                        技术选型

                                                                        技术 用途
                                                                        Astro 页面框架
                                                                        Vue 3 交互组件，组合式API
                                                                        Pinia 状态管理
                                                                        Tailwind CSS 样式
                                                                        TypeScript 类型安全

                                                                        功能模块

                                                                        模块 功能 优先级
                                                                        对话核心 文本输入、消息列表、流式输出、发送状态 P0
                                                                        语音能力 语音输入（STT）、语音输出（TTS）、播放控制 P1
                                                                        情绪可视化 情绪状态指示器、情绪变化动画 P2
                                                                        记忆查看 查看Memory Block内容（只读） P2
                                                                        基础设置 LLM配置、情绪配置、记忆容量配置 P1
                                                                        MCP市场 浏览可安装MCP、查看详情、安装 P1
                                                                        MCP管理 已安装列表、启用/禁用、配置修改、卸载 P1
                                                                        主题切换 暗色/亮色模式 P1

                                                                        页面布局

                                                                        ```
                                                                        ┌──────────────────────────────────────────────────────┐
                                                                        │  [Alice头像]  Alice         [设置] [主题]           │
                                                                        │  ─────────────────────────────────────────────────── │
                                                                        │                                                      │
                                                                        │  ┌──────────────────────────────────────────────┐   │
                                                                        │  │  消息列表                                    │   │
                                                                        │  │  [用户] 今天好累                            │   │
                                                                        │  │  [Alice] 怎么了？工作上的事吗？             │   │
                                                                        │  │  [用户] 嗯，老板又让改方案                 │   │
                                                                        │  │  [Alice] (正在输入...)                      │   │
                                                                        │  └──────────────────────────────────────────────┘   │
                                                                        │                                                      │
                                                                        │  [🎤]  [输入框...]              [发送]              │
                                                                        │                                                      │
                                                                        │  [😊 开心]  [💾 记忆: 42条]  [MCP: 3个已安装]     │
                                                                        └──────────────────────────────────────────────────────┘
                                                                        ```

                                                                        五、通信协议

                                                                        WebSocket 消息类型

                                                                        方向 类型 说明
                                                                        前端→后端 handshake 握手
                                                                        前端→后端 user_message 用户发送消息（文本/元信息）
                                                                        前端→后端 upload_chunk 上传文件分块（Binary Frame）
                                                                        前端→后端 settings_update 修改配置
                                                                        前端→后端 mcp_market_list 获取MCP市场列表
                                                                        前端→后端 mcp_install 安装MCP
                                                                        前端→后端 mcp_uninstall 卸载MCP
                                                                        前端→后端 mcp_toggle 启用/禁用MCP
                                                                        前端→后端 mcp_configure 修改MCP配置
                                                                        前端→后端 mcp_installed_list 获取已安装列表
                                                                        前端→后端 ping 心跳
                                                                        后端→前端 handshake_ack 握手确认
                                                                        后端→前端 assistant_chunk Alice回复（流式）
                                                                        后端→前端 assistant_audio Alice回复音频
                                                                        后端→前端 proactive_message 主动推送消息
                                                                        后端→前端 emotion_update 情绪状态更新
                                                                        后端→前端 mcp_capabilities MCP能力更新
                                                                        后端→前端 settings_update_ack 配置修改确认
                                                                        后端→前端 mcp_*_ack MCP操作确认
                                                                        后端→前端 error 错误
                                                                        后端→前端 pong 心跳回复

                                                                        HTTP API

                                                                        端点 方法 说明
                                                                        /api/v1/memory/block GET 获取Memory Block内容（只读）
                                                                        /api/v1/memory/block/:id GET 获取单条记忆详情
                                                                        /api/v1/memory/search POST 搜索RAG历史
                                                                        /api/v1/memory/export GET 导出所有记忆（JSON下载）

                                                                        六、技术栈汇总

                                                                        层级 技术 说明
                                                                        前端框架 Astro + Vue 3 页面+交互
                                                                        前端状态 Pinia 状态管理
                                                                        前端样式 Tailwind CSS 样式
                                                                        前端语言 TypeScript 类型安全
                                                                        后端语言 Go 高性能、并发好
                                                                        LLM OpenAI兼容API 支持任意提供商
                                                                        Embedding OpenAI / 阿里 / 本地 云端生成，固定模型
                                                                        向量数据库 Redis + RediSearch 本地内存检索
                                                                        MCP 官方 Go SDK 外设标准化接入
                                                                        MCP传输 stdio / HTTP 本地/分布式
                                                                        实时通信 WebSocket 双向通信+二进制
                                                                        配置 YAML 统一管理

                                                                        七、核心流程示例

                                                                        用户发送消息（含TTS）

                                                                        ```
                                                                        用户：今天好累（点击发送）
                                                                            │
                                                                                ▼
                                                                                前端：WebSocket发送 user_message
                                                                                    │
                                                                                        ▼
                                                                                        后端Kernel：
                                                                                            1. Emotion.process_event("user_share_bad_news")
                                                                                                   → 心疼+0.5，温柔+0.4
                                                                                                       2. RAG.retrieve("今天好累")
                                                                                                              → 查到"用户3天前说过工作压力大"
                                                                                                                  3. Memory Block.inject(rag_results)
                                                                                                                         → 新增记忆原文
                                                                                                                             4. 构建上下文：
                                                                                                                                    System Prompt + Emotion描述 + Memory Block + 用户输入
                                                                                                                                        5. MCP Client获取工具列表（TTS、STT等）
                                                                                                                                            6. LLM生成回复（带Function Calling）
                                                                                                                                                7. 如需TTS：MCP Client调用 tts_speak 工具
                                                                                                                                                    8. 存储：RAG.store(用户输入) + RAG.store(Alice回复)
                                                                                                                                                        │
                                                                                                                                                            ▼
                                                                                                                                                            后端→前端：WebSocket流式推送
                                                                                                                                                                event: assistant_chunk (内容逐字)
                                                                                                                                                                    event: assistant_chunk (done: true, 附带audio)
                                                                                                                                                                        event: emotion_update (情绪快照)
                                                                                                                                                                            │
                                                                                                                                                                                ▼
                                                                                                                                                                                前端：
                                                                                                                                                                                    1. 逐字显示回复
                                                                                                                                                                                        2. 收到audio后自动播放
                                                                                                                                                                                            3. 更新情绪指示器
                                                                                                                                                                                                4. 消息存入本地列表
                                                                                                                                                                                                ```

                                                                                                                                                                                                八、MCP生命周期流程

                                                                                                                                                                                                ```
                                                                                                                                                                                                启动时：
                                                                                                                                                                                                  Kernel启动 → MCP Manager读取配置 → 启动已启用MCP子进程
                                                                                                                                                                                                      → 建立MCP连接 → 获取工具列表 → 注册到LLM

                                                                                                                                                                                                      运行时（用户操作）：
                                                                                                                                                                                                        前端"安装MCP" → WebSocket mcp_install → MCP Manager下载二进制
                                                                                                                                                                                                            → 启动子进程 → 建立连接 → 获取工具列表 → 注册到LLM
                                                                                                                                                                                                                → 返回成功 → 前端更新市场状态

                                                                                                                                                                                                                  前端"禁用MCP" → WebSocket mcp_toggle → MCP Manager断开连接
                                                                                                                                                                                                                      → 从LLM移除工具 → 返回成功

                                                                                                                                                                                                                      用户发消息时：
                                                                                                                                                                                                                        用户输入 → Kernel构建上下文（含可用工具列表）
                                                                                                                                                                                                                            → LLM决定调用工具 → Kernel通过MCP Client调用工具
                                                                                                                                                                                                                                → 工具结果返回 → LLM生成最终回复
                                                                                                                                                                                                                                ```

                                                                                                                                                                                                                                九、设计决策汇总

                                                                                                                                                                                                                                决策点 选择 理由
                                                                                                                                                                                                                                开发语言 Go 高性能、并发好
                                                                                                                                                                                                                                前端框架 Astro+Vue 灵活、类型安全
                                                                                                                                                                                                                                实时通信 WebSocket 支持二进制+双向
                                                                                                                                                                                                                                MCP接入 仅后端 前端不感知MCP
                                                                                                                                                                                                                                MCP传输 stdio（默认） 本地部署简单
                                                                                                                                                                                                                                Embedding 云端生成，固定模型 精度优先，保证兼容
                                                                                                                                                                                                                                向量数据库 Redis 本地内存，低延迟
                                                                                                                                                                                                                                Memory Block 存原文，不压缩 LLM自然引用
                                                                                                                                                                                                                                Memory Block容量 默认无限 可配置，不丢记忆
                                                                                                                                                                                                                                记忆删除 前端不可删 保证Alice人设连续性
                                                                                                                                                                                                                                情绪模型 高维向量 可扩展、可演算
                                                                                                                                                                                                                                配置管理 YAML 不改代码

                                                                                                                                                                                                                                十、项目目录结构

                                                                                                                                                                                                                                ```
                                                                                                                                                                                                                                alice/
                                                                                                                                                                                                                                ├── frontend/                    # 前端（Astro + Vue）
                                                                                                                                                                                                                                │   ├── src/
                                                                                                                                                                                                                                │   │   ├── components/          # Vue组件
                                                                                                                                                                                                                                │   │   │   ├── chat/           # 对话相关
                                                                                                                                                                                                                                │   │   │   ├── emotion/        # 情绪可视化
                                                                                                                                                                                                                                │   │   │   ├── memory/         # 记忆查看
                                                                                                                                                                                                                                │   │   │   ├── settings/       # 设置面板
                                                                                                                                                                                                                                │   │   │   ├── mcp/            # MCP市场+管理
                                                                                                                                                                                                                                │   │   │   └── common/         # 公共组件
                                                                                                                                                                                                                                │   │   ├── pages/              # Astro页面
                                                                                                                                                                                                                                │   │   ├── stores/             # Pinia状态
                                                                                                                                                                                                                                │   │   ├── services/           # API/WebSocket服务
                                                                                                                                                                                                                                │   │   └── styles/             # 样式
                                                                                                                                                                                                                                │   └── package.json
                                                                                                                                                                                                                                │
                                                                                                                                                                                                                                ├── backend/                     # 后端（Go）
                                                                                                                                                                                                                                │   ├── kernel/                  # 核心逻辑
                                                                                                                                                                                                                                │   │   ├── kernel.go
                                                                                                                                                                                                                                │   │   ├── llm.go
                                                                                                                                                                                                                                │   │   └── context.go
                                                                                                                                                                                                                                │   ├── emotion/                 # 情绪引擎
                                                                                                                                                                                                                                │   │   ├── engine.go
                                                                                                                                                                                                                                │   │   ├── vector.go
                                                                                                                                                                                                                                │   │   └── events.go
                                                                                                                                                                                                                                │   ├── memory/                  # 记忆系统
                                                                                                                                                                                                                                │   │   ├── rag.go
                                                                                                                                                                                                                                │   │   ├── block.go
                                                                                                                                                                                                                                │   │   └── redis.go
                                                                                                                                                                                                                                │   ├── mcp/                     # MCP管理
                                                                                                                                                                                                                                │   │   ├── manager.go          # MCP生命周期管理
                                                                                                                                                                                                                                │   │   ├── client.go           # MCP客户端
                                                                                                                                                                                                                                │   │   └── registry.go         # 注册表
                                                                                                                                                                                                                                │   ├── server/                  # WebSocket/HTTP服务
                                                                                                                                                                                                                                │   │   ├── websocket.go
                                                                                                                                                                                                                                │   │   └── api.go
                                                                                                                                                                                                                                │   ├── config/                  # 配置加载
                                                                                                                                                                                                                                │   │   └── config.go
                                                                                                                                                                                                                                │   └── main.go
                                                                                                                                                                                                                                │
                                                                                                                                                                                                                                ├── mcp-server/                  # MCP Server（独立）
                                                                                                                                                                                                                                │   ├── main.go
                                                                                                                                                                                                                                │   ├── tools/
                                                                                                                                                                                                                                │   │   ├── tts.go
                                                                                                                                                                                                                                │   │   ├── stt.go
                                                                                                                                                                                                                                │   │   ├── vision.go
                                                                                                                                                                                                                                │   │   └── search.go
                                                                                                                                                                                                                                │   └── go.mod
                                                                                                                                                                                                                                │
                                                                                                                                                                                                                                ├── config/                      # 配置文件
                                                                                                                                                                                                                                │   ├── main.yaml
                                                                                                                                                                                                                                │   ├── kernel.yaml
                                                                                                                                                                                                                                │   ├── emotion.yaml
                                                                                                                                                                                                                                │   ├── emotion_events.yaml
                                                                                                                                                                                                                                │   ├── memory_rag.yaml
                                                                                                                                                                                                                                │   ├── memory_block.yaml
                                                                                                                                                                                                                                │   ├── mcp.yaml
                                                                                                                                                                                                                                │   └── prompts/
                                                                                                                                                                                                                                │       ├── system_prompt.txt
                                                                                                                                                                                                                                │       └── emotion_templates.yaml
                                                                                                                                                                                                                                │
                                                                                                                                                                                                                                ├── registry/                    # MCP注册表（本地缓存）
                                                                                                                                                                                                                                │   └── registry.json
                                                                                                                                                                                                                                │
                                                                                                                                                                                                                                ├── docs/                        # 文档
                                                                                                                                                                                                                                │   └── design.md
                                                                                                                                                                                                                                │
                                                                                                                                                                                                                                └── go.mod
                                                                                                                                                                                                                                ```

                                                                                                                                                                                                                                十一、一句话总结

                                                                                                                                                                                                                                Alice = Kernel（大脑）+ MCP Server（外设）+ Config（配置）+ Frontend（壳子）

                                                                                                                                                                                                                                · Kernel：System Prompt + Memory Block + Emotion + RAG + LLM，她是Alice的灵魂
                                                                                                                                                                                                                                · MCP Server：TTS + STT + Vision + Search + 任意可扩展工具，她是Alice的四肢
                                                                                                                                                                                                                                · MCP Manager：管理MCP生命周期，安装/启用/禁用/卸载
                                                                                                                                                                                                                                · Config：所有模块配置，不改代码
                                                                                                                                                                                                                                · Frontend：纯壳子，对话界面 + 设置面板 + MCP市场

                                                                                                                                                                                                                                用户可以通过MCP市场自由扩展Alice的能力，而她始终记得你是谁、记得你们的对话、有自己的情绪。