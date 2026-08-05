import { defineStore } from 'pinia'
import { getMCPStatus, type MCServerStatus } from '../services/api'
import { ws } from '../services/ws'

export const useMCPStore = defineStore('mcp', {
  state: () => ({
    servers: [] as MCServerStatus[],
    loading: false,
    error: '',
  }),

  actions: {
    async refresh() {
      this.loading = true
      this.error = ''
      try {
        const data = await getMCPStatus()
        this.servers = data.servers
      } catch (e: any) {
        this.error = e.message ?? '获取 MCP 状态失败'
      } finally {
        this.loading = false
      }
    },

    /** 启用/禁用 MCP Server */
    async toggle(id: string, enabled: boolean) {
      const prev = this.servers.find((s) => s.id === id)
      if (prev) prev.running = enabled // 乐观更新
      ws.send('mcp_toggle', { id, enabled })
      // 等 ack 后刷新
      await new Promise<void>((resolve) => {
        const off = ws.on('mcp_toggle_ack', (p) => {
          if (p.id === id) {
            off()
            resolve()
          }
        })
        setTimeout(resolve, 3000)
      })
      await this.refresh()
    },
  },
})
