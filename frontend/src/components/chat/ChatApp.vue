<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useChatStore } from '../../stores/chat'
import MessageList from './MessageList.vue'
import MessageInput from './MessageInput.vue'
import SettingsPanel from '../settings/SettingsPanel.vue'

const chat = useChatStore()
let unwatch: (() => void) | null = null

onMounted(() => {
  void chat.connect()
  // 新消息时自动滚到底
  unwatch = chat.$subscribe((_, state) => {
    scrollToBottom()
  })
})

onUnmounted(() => unwatch?.())
</script>

<template>
  <div class="flex h-screen flex-col bg-zinc-950 text-zinc-100">
    <!-- 顶部栏 -->
    <header class="flex items-center gap-3 border-b border-zinc-800 px-4 py-3">
      <div class="flex h-9 w-9 items-center justify-center rounded-full bg-gradient-to-br from-purple-500 to-pink-500 text-sm font-bold">
        A
      </div>
      <div class="flex-1">
        <div class="flex items-center gap-2">
          <span class="font-medium">Alice</span>
          <span
            class="inline-block h-2 w-2 rounded-full"
            :class="chat.connected ? 'bg-emerald-400' : 'bg-red-500'"
          />
          <span class="text-xs text-zinc-500">
            {{ chat.serverInfo?.llm ?? '连接中...' }}
          </span>
        </div>
      </div>
      <button
        class="rounded-md px-3 py-1.5 text-sm text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
        @click="chat.settingsOpen = true"
      >
        设置
      </button>
      <button class="rounded-md px-3 py-1.5 text-sm text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100">
        主题
      </button>
    </header>

    <!-- 消息列表 -->
    <main class="flex-1 overflow-y-auto px-4">
      <MessageList />
    </main>

    <!-- 输入区 -->
    <footer class="border-t border-zinc-800 px-4 py-3">
      <MessageInput />
      <!-- 底部状态栏（阶段占位） -->
      <div class="mt-2 flex gap-4 text-xs text-zinc-500">
        <span>😊 开心</span>
        <span>💾 记忆: {{ chat.memoryCount }} 条</span>
        <span>🔌 MCP: 0 个已安装</span>
      </div>
    </footer>

    <!-- 设置面板 -->
    <SettingsPanel v-if="chat.settingsOpen" />
  </div>
</template>
