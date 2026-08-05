import { defineStore } from 'pinia'
import { getMCPStatus, getMCPMarket, type MCServerStatus, type MarketItem } from '../services/api'
import { ws } from '../services/ws'

export const useMCPStore = defineStore('mcp', {
  state: () => ({
    servers: [] as MCServerStatus[],
    market: [] as MarketItem[],
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

    async refreshMarket() {
      try {
        const data = await getMCPMarket()
        this.market = data.items
      } catch (e: any) {
        this.error = e.message ?? '获取市场失败'
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

    /** 工具级启用/禁用 */
    async toggleTool(server: string, tool: string, enabled: boolean) {
      const srv = this.servers.find((s) => s.id === server)
      const t = srv?.tools.find((x) => x.name === tool)
      if (t) t.enabled = enabled // 乐观更新
      ws.send('mcp_tool_toggle', { server, tool, enabled })
      await new Promise<void>((resolve) => {
        const off = ws.on('mcp_tool_toggle_ack', (p) => {
          if (p.server === server && p.tool === tool) {
            off()
            resolve()
          }
        })
        setTimeout(resolve, 3000)
      })
      await this.refresh()
    },

    /** 安装 / 卸载 */
    install(id: string) {
      ws.send('mcp_install', { id })
      setTimeout(() => void this.refreshMarket(), 800)
    },

    uninstall(id: string) {
      ws.send('mcp_uninstall', { id })
      setTimeout(() => void this.refreshMarket(), 800)
    },
  },
})
