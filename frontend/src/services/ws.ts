// WebSocket 单例封装：自动重连 + 事件分发
type WsHandler = (payload: any) => void

class WsService {
  private ws: WebSocket | null = null
  private handlers = new Map<string, WsHandler[]>()
  private reconnectDelay = 1000
  private manualClose = false

  connect(url: string) {
    this.manualClose = false
    this.open(url)
  }

  private open(url: string) {
    const ws = new WebSocket(url)
    this.ws = ws

    ws.onopen = () => {
      this.reconnectDelay = 1000
      this.emit('$open', null)
      // 握手
      this.send('handshake', null)
    }

    ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data)
        this.emit(msg.type, msg.payload)
      } catch {
        // 忽略非 JSON 消息
      }
    }

    ws.onclose = () => {
      this.emit('$close', null)
      if (!this.manualClose) {
        setTimeout(() => this.open(url), this.reconnectDelay)
        this.reconnectDelay = Math.min(this.reconnectDelay * 2, 10000)
      }
    }

    ws.onerror = () => {
      ws.close()
    }
  }

  on(type: string, handler: WsHandler): () => void {
    if (!this.handlers.has(type)) this.handlers.set(type, [])
    this.handlers.get(type)!.push(handler)
    return () => this.off(type, handler)
  }

  off(type: string, handler: WsHandler) {
    const list = this.handlers.get(type)
    if (!list) return
    const idx = list.indexOf(handler)
    if (idx >= 0) list.splice(idx, 1)
  }

  private emit(type: string, payload: any) {
    for (const h of this.handlers.get(type) ?? []) {
      try {
        h(payload)
      } catch (e) {
        console.error(`[ws] handler ${type} 出错:`, e)
      }
    }
  }

  send(type: string, payload: any = null) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type, payload }))
    }
  }

  close() {
    this.manualClose = true
    this.ws?.close()
  }

  get connected() {
    return this.ws?.readyState === WebSocket.OPEN
  }
}

export const ws = new WsService()
