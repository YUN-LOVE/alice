# Alice 前端

对话界面（Astro + Vue 3 + Pinia + Tailwind CSS）。纯壳子：所有智能在后端，前端只做展示与交互。

## 开发

```bash
npm install
npm run dev        # http://localhost:4321
npm run build      # 构建产物在 dist/
```

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
│   └── settings/    # 设置面板（后端地址）
├── services/
│   ├── ws.ts        # WebSocket 封装（自动重连）
│   └── backend.ts   # 后端地址解析
├── stores/          # Pinia（chat store）
└── pages/           # Astro 页面
```

## 协议

与后端通信协议见 [docs/api.md](../docs/api.md)。
