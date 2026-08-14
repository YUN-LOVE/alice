<script setup lang="ts">
import { useMemoryStore } from '../../stores/memory'

const memory = useMemoryStore()

function roleLabel(role: string) {
  return role === 'user' ? '你' : role === 'assistant' ? 'Alice' : '记忆'
}

function roleIcon(role: string) {
  return role === 'user' ? 'person' : role === 'assistant' ? 'smart_toy' : 'database'
}

// create_at 来自 Memory Block API：RFC3339 字符串（如 "2026-08-14T10:00:00+08:00"），
// 兼容 Unix 秒（历史记录 API）两种格式
function fmtTime(v: string | number): string {
  if (!v) return ''
  const d = typeof v === 'number' ? new Date(v * 1000) : new Date(v)
  if (Number.isNaN(d.getTime())) return ''
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  return `${hh}:${mm}`
}
</script>

<template>
  <div class="flex flex-col gap-3">
    <!-- 搜索：M3 填充式输入框 -->
    <div class="m3-field">
      <div class="m3-field__box">
        <span class="m3-field__leading m3-icon">search</span>
        <input
          v-model="memory.searchQuery"
          type="text"
          class="m3-field__input"
          placeholder="搜索历史记忆..."
          @keyup.enter="memory.search(memory.searchQuery)"
        />
      </div>
    </div>

    <div v-if="memory.error" class="m3-label-medium text-[var(--md-sys-color-error)]">{{ memory.error }}</div>

    <!-- 搜索结果 -->
    <div v-if="memory.searchResults.length > 0" class="flex flex-col gap-2">
      <div class="m3-section-title">搜索结果 ({{ memory.searchResults.length }})</div>
      <div
        v-for="r in memory.searchResults"
        :key="r.mem.id"
        class="m3-card m3-card--outlined"
      >
        <div class="m3-label-small m3-on-surface-variant">
          {{ roleLabel(r.mem.role) }} · 相似度 {{ (r.score * 100).toFixed(0) }}%
        </div>
        <p class="m3-body-medium m3-on-surface mt-1.5">{{ r.mem.text }}</p>
      </div>
      <button class="m3-btn m3-btn--text m3-state-layer m3-ripple m3-btn--sm self-start" @click="memory.searchResults = []">
        返回全部记忆
      </button>
    </div>

    <!-- Memory Block 列表 -->
    <div v-else class="flex flex-col gap-2">
      <div class="flex items-center justify-between">
        <span class="m3-section-title !p-0">短期工作记忆 ({{ memory.total }})</span>
        <button
          class="m3-icon-btn m3-icon-btn--xs m3-state-layer m3-ripple"
          title="刷新"
          @click="memory.refresh()"
        >
          <span class="m3-icon m3-icon--sm">refresh</span>
        </button>
      </div>
      <div v-if="memory.loading" class="flex items-center gap-2 py-2">
        <span class="m3-spinner h-4 w-4" />
        <span class="m3-label-medium m3-on-surface-variant">加载中...</span>
      </div>
      <div v-else-if="memory.entries.length === 0" class="m3-body-medium m3-on-surface-variant py-6 text-center">
        还没有对话记忆，聊几句就有了
      </div>

      <!-- M3 列表 -->
      <div class="m3-list">
        <div
          v-for="e in [...memory.entries].reverse()"
          :key="e.id"
          class="m3-list-item m3-list-item--two-line m3-state-layer"
        >
          <div class="m3-list-item__leading" :class="e.role === 'assistant' ? 'm3-avatar--alice' : ''">
            <span class="m3-icon" :class="e.role === 'assistant' ? '!text-[var(--md-sys-color-on-primary)]' : ''">{{ roleIcon(e.role) }}</span>
          </div>
          <div class="m3-list-item__text">
            <span class="m3-list-item__title">{{ roleLabel(e.role) }}</span>
            <span class="m3-list-item__subtitle line-clamp-2 whitespace-pre-wrap">{{ e.text }}</span>
          </div>
          <span class="m3-list-item__trailing m3-label-small m3-on-surface-variant">
            {{ fmtTime(e.create_at) }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>
