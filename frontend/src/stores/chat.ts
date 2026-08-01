import { defineStore, createPinia, setActivePinia } from 'pinia'
import { ws } from '../services/ws'
import { ensureConfig, getBackendUrl, setBackendUrl, wsUrl, httpBase } from '../services/backend'

export interface ChatMessage {
  id: number
  role: 'user' | 'assistant'
  content: string
  streaming?: boolean
}

// 非组件上下文使用 Pinia：全局单一实例
setActivePinia(createPinia())

let msgId = 0

export const useChatStore = defineStore('chat', {
  state: () => ({
    messages: [] as ChatMessage[],
    connected: false,
    sending: false,
    serverInfo: null as { name: string; llm: string; version: string } | null,
    backendUrl: '',
    memoryCount: 0,
    settingsOpen: false,
    initialized: false,
  }),

  actions: {
    async init() {
      if (this.initialized) return
      this.initialized = true
      await ensureConfig()
      this.backendUrl = getBackendUrl()

      ws.on('$open', () => {
        this.connected = true
      })
      ws.on('$close', () => {
        this.connected = false
      })
      ws.on('handshake_ack', (p) => {
        this.serverInfo = p
      })
      ws.on('assistant_chunk', (p) => {
        this.onAssistantChunk(p)
      })
      ws.on('error', (p) => {
        this.pushAssistant(p.message ?? '出错了')
      })

      // 拉取记忆条数（状态栏展示）
      void this.refreshMemoryCount()
    },

    async refreshMemoryCount() {
      try {
        const resp = await fetch(`${httpBase()}/api/v1/memory/block`)
        const data = await resp.json()
        this.memoryCount = data.total ?? 0
      } catch {
        // 后端不可用时保持 0
      }
    },

    async connect(url?: string) {
      await this.init()
      if (url) {
        setBackendUrl(url)
        this.backendUrl = getBackendUrl()
      }
      ws.connect(wsUrl())
    },

    /** 更换后端地址并重连 */
    reconnectTo(url: string) {
      ws.close()
      this.serverInfo = null
      void this.connect(url)
    },

    sendText(text: string) {
      if (!text.trim() || this.sending) return
      this.messages.push({ id: ++msgId, role: 'user', content: text })
      this.messages.push({ id: ++msgId, role: 'assistant', content: '', streaming: true })
      this.sending = true
      ws.send('user_message', { text })
    },

    onAssistantChunk(p: { content?: string; done?: boolean }) {
      const last = this.messages[this.messages.length - 1]
      if (last && last.role === 'assistant' && last.streaming) {
        if (p.content) last.content += p.content
        if (p.done) {
          last.streaming = false
          this.sending = false
        }
      }
    },

    pushAssistant(text: string) {
      this.messages.push({ id: ++msgId, role: 'assistant', content: text })
      this.sending = false
    },

    reset() {
      this.messages = []
      this.sending = false
    },
  },
})
