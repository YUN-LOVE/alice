# Alice 构建辅助
.PHONY: all backend mcp frontend dev redis clean

# 全量构建：内置 MCP + 后端
all: mcp backend

# 后端（Go）
backend:
	cd backend && go build -o alice-server .

# 内置 MCP Server（本地工具 + 视觉 + 语音合成 + 语音识别）
mcp:
	cd mcp-server && go build -o alice-local-tools .
	cd mcp-vision && go build -o alice-vision-tools .
	cd mcp-tts && go build -o alice-tts .
	cd mcp-stt && go build -o alice-stt .

# 前端（pnpm）
frontend:
	cd frontend && pnpm install && pnpm build

# 前端开发服务器
dev:
	cd frontend && pnpm dev

# 启动 Redis（未安装时先安装：pacman -S redis / apt install redis-server）
redis:
	redis-server --daemonize yes --port 6379

clean:
	rm -f backend/alice-server mcp-server/alice-local-tools mcp-vision/alice-vision-tools mcp-tts/alice-tts mcp-stt/alice-stt
