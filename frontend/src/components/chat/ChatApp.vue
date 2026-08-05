<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useChatStore } from '../../stores/chat'
import { useMCPStore } from '../../stores/mcp'
import MessageList from './MessageList.vue'
import MessageInput from './MessageInput.vue'
import SettingsPanel from '../settings/SettingsPanel.vue'
import SidePanel from '../panel/SidePanel.vue'

const chat = useChatStore()
const mcp = useMCPStore()
const scrollContainer = ref<HTMLElement | null>(null)
let unwatch: (() => void) | null = null

async function scrollToBottom() {
  await nextTick()
  if (scrollContainer.value) {
    scrollContainer.value.scrollTo({ top: scrollContainer.value.scrollHeight })
  }
}

function openPanel(tab: 'memory' | 'mcp' | 'emotion') {
  chat.panelTab = tab
  chat.panelOpen = true
}

onMounted(() => {
  void chat.connect()
  void chat.loadHistory() // 加载当天历史聊天记录
  void mcp.refresh() // 状态栏显示 MCP 数量
  // 新消息时自动滚到底
  unwatch = chat.$subscribe(() => {
    void scrollToBottom()
  })
})

onUnmounted(() => unwatch?.())
</script>

<template>
  <div class="m3-surface relative flex h-screen flex-col">
    <!-- 顶部栏 -->
    <header class="m3-surface-container-low flex items-center gap-3 border-b border-zinc-800 px-4 py-3">
      <div
        class="flex h-10 w-10 items-center justify-center rounded-full bg-gradient-to-br from-purple-500 to-pink-500 text-base font-bold m3-on-primary shadow-lg"
      >
        A
      </div>
      <div class="flex-1">
        <div class="flex items-center gap-2">
          <span class="font-medium">Alice</span>
          <span
            class="inline-block h-2 w-2 rounded-full"
            :class="chat.connected ? 'bg-emerald-400' : 'bg-red-500'"
          />
          <span class="text-xs m3-on-surface-variant">
            {{ chat.serverInfo?.llm ?? '连接中...' }}
          </span>
        </div>
      </div>
      <button
        class="rounded-full px-3.5 py-1.5 text-sm m3-on-surface-variant hover:bg-zinc-800"
        @click="openPanel('memory')"
      >
        记忆
      </button>
      <button
        class="rounded-full px-3.5 py-1.5 text-sm m3-on-surface-variant hover:bg-zinc-800"
        @click="openPanel('mcp')"
      >
        MCP
      </button>
      <button
        class="rounded-full px-3.5 py-1.5 text-sm m3-on-surface-variant hover:bg-zinc-800"
        @click="openPanel('emotion')"
      >
        情绪
      </button>
      <button
        class="rounded-full px-3.5 py-1.5 text-sm m3-on-surface-variant hover:bg-zinc-800"
        @click="chat.settingsOpen = true"
      >
        设置
      </button>
      <button
        class="rounded-full px-3 py-1.5 text-sm m3-on-surface-variant hover:bg-zinc-800"
        @click="chat.toggleTheme()"
      >
        {{ chat.theme === 'dark' ? '☀️' : '🌙' }}
      </button>
    </header>

    <!-- 消息列表 -->
    <main ref="scrollContainer" class="flex-1 overflow-y-auto px-4">
      <MessageList />
    </main>

    <!-- 输入区 -->
    <footer class="m3-surface-container-low border-t border-zinc-800 px-4 py-3">
      <MessageInput />
      <!-- 底部状态栏 -->
      <div class="mt-2 flex gap-4 text-xs m3-on-surface-variant">
        <span>😊 {{ chat.emotion.top || '平静' }}</span>
        <span>💾 记忆: {{ chat.memoryCount }} 条</span>
        <span>🔌 MCP: {{ mcp.servers.filter((s) => s.running).length }} 个运行中</span>
      </div>
    </footer>

    <!-- 侧边面板 -->
    <SidePanel />

    <!-- 设置面板 -->
    <SettingsPanel v-if="chat.settingsOpen" />
  </div>
</template>
