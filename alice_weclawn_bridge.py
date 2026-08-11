import asyncio
import json
import uuid
import os
import logging
from fastapi import FastAPI, Request
import websockets

# ========== 配置 ==========
ALICE_WS = os.getenv("ALICE_WS", "ws://127.0.0.1:8081/ws")
WECLAW_API = os.getenv("WECLAW_API", "http://127.0.0.1:18011/api/send")
WECLAW_KEY = os.getenv("WECLAW_KEY", "your-weclaw-api-key")
WECLAW_DEFAULT_USER = os.getenv("WECLAW_USER", "default_wechat_id")
# ==========================

app = FastAPI()
logging.basicConfig(level=logging.INFO)

async def proactive_listener():
    """后台监听主动推送（保持不变）"""
    while True:
        try:
            async with websockets.connect(ALICE_WS) as ws:
                await ws.send(json.dumps({
                    "type": "handshake",
                    "payload": {"session_id": "weclaw_bridge"}
                }))
                await ws.recv()
                logging.info("Proactive listener ready")

                while True:
                    msg = await ws.recv()
                    data = json.loads(msg)
                    
                    if data.get("type") == "ping":
                        await ws.send(json.dumps({
                            "type": "pong",
                            "payload": {"time": data.get("payload", {}).get("time")}
                        }))
                        continue

                    if data.get("type") == "proactive_message":
                        text = data.get("payload", {}).get("text", "")
                        logging.info(f"Proactive: {text}")
                        # 主动推送保持原样（这里依赖外部脚本调用 WeClaw API）
                        # 实际生产推荐单独开一个 HTTP 端点来触达 WeClaw
        except Exception as e:
            logging.error(f"Proactive listener crashed: {e}, retry in 5s")
            await asyncio.sleep(5)

@app.on_event("startup")
async def startup():
    asyncio.create_task(proactive_listener())

# ========== 核心对话接口（纯 JSON 响应，无 SSE） ==========
@app.post("/v1/chat/completions")
async def chat_completions(request: Request):
    body = await request.json()
    user_id = body.get("user", "wechat_guest")
    messages = body.get("messages", [])
    if not messages:
        return {
            "id": f"chatcmpl-{uuid.uuid4().hex[:8]}",
            "object": "chat.completion",
            "choices": [{"index": 0, "message": {"role": "assistant", "content": "请发送消息"}, "finish_reason": "stop"}]
        }
    
    user_text = messages[-1]["content"]
    resp_id = f"chatcmpl-{uuid.uuid4().hex[:8]}"

    try:
        async with websockets.connect(ALICE_WS) as ws:
            # 1. 握手
            await ws.send(json.dumps({
                "type": "handshake",
                "payload": {"session_id": user_id}
            }))
            await ws.recv()  # 忽略 handshake_ack

            # 2. 发消息
            await ws.send(json.dumps({
                "type": "user_message",
                "payload": {"text": user_text, "session_id": user_id}
            }))

            full_content = ""
            while True:
                try:
                    msg = await asyncio.wait_for(ws.recv(), timeout=60.0)
                except (asyncio.TimeoutError, websockets.ConnectionClosed):
                    break

                data = json.loads(msg)
                msg_type = data.get("type")

                # 处理心跳 Ping（必须回复，否则后端可能断连）
                if msg_type == "ping":
                    await ws.send(json.dumps({
                        "type": "pong",
                        "payload": {"time": data.get("payload", {}).get("time")}
                    }))
                    continue

                # 核心：攒 chunk
                if msg_type == "assistant_chunk":
                    payload = data.get("payload", {})
                    content = payload.get("content", "")
                    done = payload.get("done", False)
                    if content:
                        full_content += content
                    if done:
                        break

                # 错误透传
                if msg_type == "error":
                    full_content = f"❌ {data.get('payload', {}).get('message', '未知错误')}"
                    break

            # 返回标准 OpenAI 非流式 JSON
            return {
                "id": resp_id,
                "object": "chat.completion",
                "choices": [{
                    "index": 0,
                    "message": {"role": "assistant", "content": full_content or "（没有收到回复）"},
                    "finish_reason": "stop"
                }]
            }

    except Exception as e:
        logging.exception("Bridge error")
        # 出错也返回合规 JSON
        return {
            "id": resp_id,
            "object": "chat.completion",
            "choices": [{
                "index": 0,
                "message": {"role": "assistant", "content": f"⚠️ 系统错误: {str(e)}"},
                "finish_reason": "stop"
            }]
        }

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=9090)
