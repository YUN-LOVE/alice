// HTTP API 封装（后端解耦，基于 backend.ts 推导的地址）
import { httpBase } from './backend'

export interface BlockEntry {
  id: number
  role: 'user' | 'assistant' | 'memory'
  text: string
  source: string
  create_at: string
}

export interface MCServerStatus {
  id: string
  name: string
  enabled: boolean
  running: boolean
  tool_count: number
  tools: { name: string; enabled: boolean }[]
}

export interface MarketItem {
  id: string
  name: string
  description: string
  installed: boolean
}

export interface SearchResult {
  mem: {
    id: number
    role: string
    text: string
    create_at: string
  }
  score: number
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(`${httpBase()}/api/v1${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  if (!resp.ok) {
    const data = await resp.json().catch(() => null)
    throw new Error(data?.message ?? `请求失败: ${resp.status}`)
  }
  return resp.json()
}

export function getMemoryBlock(): Promise<{ total: number; entries: BlockEntry[] }> {
  return request('/memory/block')
}

export function searchMemory(query: string): Promise<{ results: SearchResult[] }> {
  return request('/memory/search', {
    method: 'POST',
    body: JSON.stringify({ query }),
  })
}

export function getMCPStatus(): Promise<{ servers: MCServerStatus[] }> {
  return request('/mcp/status')
}

export function getMCPMarket(): Promise<{ items: MarketItem[] }> {
  return request('/mcp/market')
}

export function getHealth(): Promise<{ status: string; llm: string; embedding: string; memory: number }> {
  return request('/health')
}

export function getProactiveEnabled(): Promise<{ enabled: boolean }> {
  return request('/emotion/proactive')
}

export function setProactiveEnabled(enabled: boolean): Promise<{ enabled: boolean }> {
  return request('/emotion/proactive', {
    method: 'POST',
    body: JSON.stringify({ enabled }),
  })
}

export interface HistoryMessage {
  role: string
  content: string
  create_at: number
}

/** 当天聊天记录（历史，零点后归档到 RAG，只显示当天） */
export function getHistory(date?: string): Promise<{ date: string; messages: HistoryMessage[] }> {
  const q = date ? `?date=${date}` : ''
  return request(`/history${q}`)
}
