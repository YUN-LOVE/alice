<script setup lang="ts">
import { useMemoryStore } from '../../stores/memory'

const memory = useMemoryStore()

function roleLabel(role: string) {
  return role === 'user' ? '你' : role === 'assistant' ? 'Alice' : '记忆'
}
</script>

<template>
  <div class="flex h-full flex-col gap-3">
    <!-- 搜索 -->
    <div>
      <input
        v-model="memory.searchQuery"
        type="text"
        placeholder="搜索历史记忆..."
        class="w-full rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 placeholder-zinc-600 focus:border-purple-500 focus:outline-none"
        @keyup.enter="memory.search(memory.searchQuery)"
      />
    </div>

    <div v-if="memory.error" class="text-xs text-red-400">{{ memory.error }}</div>

    <!-- 搜索结果 -->
    <div v-if="memory.searchResults.length > 0" class="flex flex-col gap-2">
      <div class="text-xs text-zinc-500">搜索结果 ({{ memory.searchResults.length }})</div>
      <div
        v-for="r in memory.searchResults"
        :key="r.mem.id"
        class="rounded-lg border border-zinc-800 bg-zinc-950 p-3"
      >
        <div class="text-xs text-zinc-500">
          {{ roleLabel(r.mem.role) }} · 相似度 {{ (r.score * 100).toFixed(0) }}%
        </div>
        <p class="mt-1 text-sm text-zinc-200">{{ r.mem.text }}</p>
      </div>
      <button class="text-left text-xs text-purple-400 hover:underline" @click="memory.searchResults = []">
        返回全部记忆
      </button>
    </div>

    <!-- Memory Block 列表 -->
    <div v-else class="flex flex-1 flex-col gap-2 overflow-y-auto">
      <div class="flex items-center justify-between">
        <span class="text-xs text-zinc-500">短期工作记忆 ({{ memory.total }})</span>
        <button class="text-xs text-zinc-500 hover:text-zinc-300" @click="memory.refresh()">刷新</button>
      </div>
      <div v-if="memory.loading" class="text-xs text-zinc-500">加载中...</div>
      <div v-else-if="memory.entries.length === 0" class="text-xs text-zinc-600">
        还没有对话记忆，聊几句就有了
      </div>
      <div
        v-for="e in [...memory.entries].reverse()"
        :key="e.id"
        class="rounded-lg border border-zinc-800 bg-zinc-950 p-3"
      >
        <div class="text-xs text-zinc-500">
          {{ roleLabel(e.role) }}
          <span class="text-zinc-700">· {{ new Date(e.create_at * 1000).toLocaleTimeString() }}</span>
        </div>
        <p class="mt-1 whitespace-pre-wrap text-sm text-zinc-200">{{ e.text }}</p>
      </div>
    </div>
  </div>
</template>
