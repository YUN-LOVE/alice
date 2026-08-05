# Alice 前端

对话界面（Astro + Vue 3 + Pinia + Tailwind CSS）。纯壳子：所有智能在后端，前端只做展示与交互。

## 开发

```bash
pnpm install
pnpm dev        # http://localhost:4321（监听 0.0.0.0，局域网可访问）
pnpm build      # 构建产物在 dist/
```

## 功能

- 对话：流式输出、发送状态、历史聊天记录加载（当天）
- **Markdown 渲染**：消息按 GFM 渲染（marked + DOMPurify 防 XSS），流式中保持纯文本
- 侧边面板：记忆查看（Block 列表 + RAG 搜索）、MCP 管理（工具级开关）/ 市场（安装/卸载）、情绪可视化
- 主题切换：暗色 / 亮色（localStorage 持久化）
- 设置：后端地址配置（运行时切换）

## 配置后端地址

前端与后端完全解耦，只知道一个后端地址（HTTP 形式）。优先级：

1. **运行时设置**：页面右上角「设置」→ 输入地址 → 保存并重连（存 localStorage）
2. **部署文件**：编辑 `public/config.json` 的 `backendUrl`
3. **自动推导**：开发模式默认 `http://localhost:8081`；同源部署用当前站点地址

WebSocket 地址由后端地址自动推导：`http://…` → `ws://…/ws`。

## 目录

```
src/
├── components/
│   ├── chat/        # 对话（消息列表 / 输入框 / 根组件）
│   ├── settings/    # 设置面板（后端地址）
│   ├── panel/       # 侧边面板容器（记忆/MCP/情绪 tab）
│   ├── memory/      # 记忆查看面板
│   ├── mcp/         # MCP 管理 + 市场面板
│   └── emotion/     # 情绪可视化面板
├── services/
│   ├── ws.ts        # WebSocket 封装（自动重连）
│   ├── backend.ts   # 后端地址解析
│   ├── api.ts       # HTTP API 封装（记忆/MCP/历史）
│   └── markdown.ts  # Markdown 渲染 + XSS 净化
├── stores/          # Pinia（chat / mcp / memory）
└── pages/           # Astro 页面
```

## 协议

与后端通信协议见 [docs/api.md](../docs/api.md)。
