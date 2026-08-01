// 后端地址解析：localStorage（运行时设置） > config.json > 自动推导
// 前端与后端完全解耦：只认这一个地址，HTTP 和 WebSocket 都由它推导

const LS_KEY = 'alice.backendUrl'
let fileConfig: { backendUrl?: string } | null = null

export async function ensureConfig(): Promise<void> {
  if (fileConfig) return
  try {
    const resp = await fetch('./config.json')
    fileConfig = await resp.json()
  } catch {
    fileConfig = {}
  }
}

export function getBackendUrl(): string {
  const stored = localStorage.getItem(LS_KEY)
  if (stored) return stored
  if (fileConfig?.backendUrl) return fileConfig.backendUrl
  return deriveDefault()
}

export function setBackendUrl(url: string): void {
  if (url.trim()) {
    localStorage.setItem(LS_KEY, url.trim().replace(/\/+$/, ''))
  } else {
    localStorage.removeItem(LS_KEY)
  }
}

export function wsUrl(): string {
  return getBackendUrl().replace(/^http/i, 'ws') + '/ws'
}

export function httpBase(): string {
  return getBackendUrl()
}

// 开发模式（Astro dev :4321）默认本机后端；否则假定同源反代部署
function deriveDefault(): string {
  if (import.meta.env.DEV) return 'http://localhost:8081'
  return location.origin
}
