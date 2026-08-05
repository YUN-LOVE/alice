<script setup lang="ts">
import { watch } from 'vue'
import { useChatStore } from '../../stores/chat'
import { useMemoryStore } from '../../stores/memory'
import { useMCPStore } from '../../stores/mcp'
import MemoryPanel from '../memory/MemoryPanel.vue'
import MCPPanel from '../mcp/MCPPanel.vue'
import EmotionPanel from '../emotion/EmotionPanel.vue'

const chat = useChatStore()
const memory = useMemoryStore()
const mcp = useMCPStore()

const tabs = [
  { id: 'memory', label: '记忆' },
  { id: 'mcp', label: 'MCP' },
  { id: 'emotion', label: '情绪' },
]

// 切换 tab 时懒加载对应数据
watch(
  () => chat.panelOpen && chat.panelTab,
  (v) => {
    if (v === 'memory') void memory.refresh()
    if (v === 'mcp') void mcp.refresh()
  }
)
</script>

<template>
  <Transition
    enter-active-class="transition duration-200 ease-out"
    enter-from-class="translate-x-full"
    leave-active-class="transition duration-200 ease-in"
    leave-to-class="translate-x-full"
  >
    <aside
      v-if="chat.panelOpen"
      class="m3-surface-container absolute bottom-0 right-0 top-14 z-40 flex w-[340px] flex-col border-l border-zinc-800"
    >
      <div class="flex items-center justify-between border-b border-zinc-800 px-4 py-3">
        <div class="flex gap-1">
          <button
            v-for="t in tabs"
            :key="t.id"
            class="rounded-full px-3 py-1.5 text-sm transition"
            :class="chat.panelTab === t.id ? 'm3-secondary-container' : 'm3-on-surface-variant hover:opacity-80'"
            @click="chat.panelTab = t.id"
          >
            {{ t.label }}
          </button>
        </div>
        <button class="m3-on-surface-variant hover:opacity-80" @click="chat.panelOpen = false">✕</button>
      </div>

      <div class="flex-1 overflow-y-auto p-4">
        <MemoryPanel v-if="chat.panelTab === 'memory'" />
        <MCPPanel v-if="chat.panelTab === 'mcp'" />
        <EmotionPanel v-if="chat.panelTab === 'emotion'" />
      </div>
    </aside>
  </Transition>
</template>
