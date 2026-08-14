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

/** 当前情绪状态（面板加载用） */
export function getEmotionState(): Promise<{
  state: Record<string, number>
  description: string
  top: string
}> {
  return request('/emotion/state')
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

export interface RuntimeSettings {
  llm: {
    provider: string
    base_url: string
    api_key_configured: boolean
    model: string
    temperature: number
    max_tokens: number
  }
  emotion: {
    decay_rate: number
    max_value: number
    threshold: number
    cooldown_seconds: number
    tick_seconds: number
    silent_after_minutes: number
    skip_if_active_minutes: number
    hours: number[]
  }
  block: { max_entries: number }
  audio: {
    tts_enabled: boolean
    tts_voice: string
    tts_rate: string
    tts_pitch: string
    tts_volume: string
  }
}

/** 当前可调设置（设置面板加载用） */
export function getSettings(): Promise<RuntimeSettings> {
  return request('/settings')
}

export interface VoiceInfo {
  short_name: string
  friendly_name: string
  gender: string
  locale: string
  status: number
}

/** TTS 可用音色列表（?locale=zh 可选过滤） */
export function getVoices(locale?: string): Promise<{ voices: VoiceInfo[] }> {
  const q = locale ? `?locale=${encodeURIComponent(locale)}` : ''
  return request(`/audio/voices${q}`)
}

/** TTS 试听合成 → {url, duration_sec} */
export function ttsPreview(params: {
  text: string
  voice?: string
  rate?: string
  pitch?: string
  volume?: string
}): Promise<{ url: string; duration_sec: number }> {
  return request('/audio/tts', {
    method: 'POST',
    body: JSON.stringify(params),
  })
}
