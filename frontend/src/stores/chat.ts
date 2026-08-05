import { defineStore, createPinia, setActivePinia } from 'pinia'
import { ws } from '../services/ws'
import { ensureConfig, getBackendUrl, setBackendUrl, wsUrl, httpBase } from '../services/backend'
import { getHistory, getProactiveEnabled, setProactiveEnabled } from '../services/api'
import {
  getSavedTheme,
  saveTheme,
  applyThemeFromSeed,
  applyThemeFromImage,
  type ThemeState,
  type ThemeStyle,
} from '../styles/theme'

export interface ChatMessage {
  id: number
  role: 'user' | 'assistant'
  content: string
  streaming?: boolean
  time?: number // 毫秒时间戳
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
    panelOpen: false,
    panelTab: 'memory' as 'memory' | 'mcp' | 'emotion',
    theme: 'dark' as 'dark' | 'light',
    seedColor: '#7c5cff',
    themeStyle: 'tonal-spot' as ThemeStyle,
    initialized: false,
    emotion: {
      top: '',
      state: {} as Record<string, number>,
    },
    proactiveEnabled: true,
  }),

  actions: {
    async init() {
      if (this.initialized) return
      this.initialized = true
      await ensureConfig()
      this.backendUrl = getBackendUrl()

      // 恢复主题（M3）
      const savedTheme = getSavedTheme()
      this.theme = savedTheme.mode
      this.seedColor = savedTheme.seed
      this.themeStyle = savedTheme.style
      this.applyTheme()

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
      ws.on('emotion_update', (p) => {
        this.emotion = {
          top: p.top ?? '',
          state: p.state ?? {},
        }
      })
      ws.on('proactive_message', (p) => {
        // 主动推送：作为 Alice 的消息展示
        this.pushAssistant(p.text ?? '')
      })
      ws.on('error', (p) => {
        this.pushAssistant(p.message ?? '出错了')
      })

      // 拉取记忆条数（状态栏展示）
      void this.refreshMemoryCount()
      void this.loadProactiveEnabled()
    },

    async loadProactiveEnabled() {
      try {
        const data = await getProactiveEnabled()
        this.proactiveEnabled = data.enabled
      } catch {
        // 忽略
      }
    },

    async toggleProactive() {
      const next = !this.proactiveEnabled
      this.proactiveEnabled = next
      try {
        await setProactiveEnabled(next)
      } catch {
        this.proactiveEnabled = !next
      }
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

    /** 加载当天历史聊天记录（零点后归档到 RAG，前端只展示当天） */
    async loadHistory() {
      try {
        const data = await getHistory()
        if (!data.messages.length) return
        const historyIdBase = 100000
        this.messages = data.messages.map((m, i) => ({
          id: historyIdBase + i,
          role: m.role === 'assistant' ? 'assistant' : 'user',
          content: m.content,
          time: m.create_at * 1000,
        }))
        msgId = historyIdBase + data.messages.length
      } catch {
        // 后端不可用时静默
      }
    },

    /** 更换后端地址并重连 */
    reconnectTo(url: string) {
      ws.close()
      this.serverInfo = null
      void this.connect(url)
    },

    sendText(text: string) {
      if (!text.trim() || this.sending) return
      const now = Date.now()
      this.messages.push({ id: ++msgId, role: 'user', content: text, time: now })
      this.messages.push({ id: ++msgId, role: 'assistant', content: '', streaming: true, time: now })
      this.sending = true
      ws.send('user_message', { text })
    },

    onAssistantChunk(p: { content?: string; done?: boolean }) {
      const last = this.messages[this.messages.length - 1]
      if (last && last.role === 'assistant' && last.streaming) {
        if (p.content) last.content += p.content
        if (p.done) {
          last.streaming = false
          last.time = Date.now()
          this.sending = false
        }
      }
    },

    pushAssistant(text: string) {
      this.messages.push({ id: ++msgId, role: 'assistant', content: text, time: Date.now() })
      this.sending = false
    },

    toggleTheme() {
      this.theme = this.theme === 'dark' ? 'light' : 'dark'
      this.applyTheme()
    },

    /** 应用 M3 主题：seed + 取色算法 → CSS 变量 */
    applyTheme() {
      applyThemeFromSeed(this.seedColor, this.themeStyle, this.theme)
      document.documentElement.classList.toggle('light', this.theme === 'light')
      document.body.style.background = 'var(--md-sys-color-surface)'
      saveTheme({ seed: this.seedColor, mode: this.theme, style: this.themeStyle })
    },

    /** 设置种子色（手动选色） */
    setSeedColor(seed: string) {
      this.seedColor = seed
      this.applyTheme()
    },

    /** 切换取色算法 */
    setThemeStyle(style: ThemeStyle) {
      this.themeStyle = style
      this.applyTheme()
    },

    /** 从壁纸图片取色生成主题（Monet），返回提取的种子色 */
    async applyWallpaper(source: string | HTMLImageElement): Promise<string> {
      const seed = await applyThemeFromImage(source, this.themeStyle, this.theme)
      this.seedColor = seed
      document.documentElement.classList.toggle('light', this.theme === 'light')
      document.body.style.background = 'var(--md-sys-color-surface)'
      saveTheme({ seed, mode: this.theme, style: this.themeStyle })
      return seed
    },

    reset() {
      this.messages = []
      this.sending = false
    },
  },
})
