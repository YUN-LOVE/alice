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
const keyboardInset = ref(0) // 虚拟键盘遮挡高度（旧浏览器兼容，键盘弹出时把输入区顶起）
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

// 移动端虚拟键盘适配：
// - 支持 interactive-widget=resizes-content 的浏览器，布局视口随键盘收缩，dvh 自动变小，无需额外处理
// - 旧浏览器（键盘覆盖内容）通过 visualViewport 计算遮挡高度，垫在输入区下方，让输入框浮在键盘上方
function onKeyboardChange() {
  const vv = window.visualViewport
  const vh = vv?.height ?? window.innerHeight
  const diff = window.innerHeight - vh
  keyboardInset.value = diff > 0 ? diff : 0
  void scrollToBottom()
}

onMounted(() => {
  const vv = window.visualViewport
  if (vv) {
    vv.addEventListener('resize', onKeyboardChange)
    vv.addEventListener('scroll', onKeyboardChange)
  }
  void chat.connect()
  void chat.loadHistory() // 加载当天历史聊天记录
  void mcp.refresh() // 状态栏显示 MCP 数量
  // 新消息时自动滚到底
  unwatch = chat.$subscribe(() => {
    void scrollToBottom()
  })
})

onUnmounted(() => {
  const vv = window.visualViewport
  vv?.removeEventListener('resize', onKeyboardChange)
  vv?.removeEventListener('scroll', onKeyboardChange)
  unwatch?.()
})
</script>

<template>
  <div class="m3-surface relative flex h-dvh flex-col overflow-hidden">
    <!-- 顶部栏 -->
    <header class="m3-surface-container-low flex h-16 shrink-0 items-center gap-2 border-b border-zinc-500 px-3 sm:gap-3 sm:px-4">
      <div
        class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-purple-500 to-pink-500 text-base font-bold m3-on-primary shadow-lg"
      >
        A
      </div>
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <span class="shrink-0 font-medium">Alice</span>
          <span
            class="inline-block h-2 w-2 shrink-0 rounded-full"
            :class="chat.connected ? 'bg-emerald-400' : 'bg-red-500'"
          />
          <span class="hidden min-w-0 truncate text-xs sm:inline m3-on-surface-variant">
            {{ chat.serverInfo?.llm ?? '连接中...' }}
          </span>
        </div>
      </div>

      <!-- 桌面端快捷入口：记忆 / MCP / 情绪 -->
      <nav class="hidden shrink-0 items-center gap-1 md:flex">
        <button
          class="rounded-full px-3 py-1.5 text-sm m3-on-surface-variant hover:bg-zinc-800"
          @click="openPanel('memory')"
        >
          记忆
        </button>
        <button
          class="rounded-full px-3 py-1.5 text-sm m3-on-surface-variant hover:bg-zinc-800"
          @click="openPanel('mcp')"
        >
          MCP
        </button>
        <button
          class="rounded-full px-3 py-1.5 text-sm m3-on-surface-variant hover:bg-zinc-800"
          @click="openPanel('emotion')"
        >
          情绪
        </button>
      </nav>

      <!-- 移动端菜单：打开侧边面板 -->
      <button
        class="rounded-full p-2 text-lg m3-on-surface-variant hover:bg-zinc-800 md:hidden"
        title="菜单（记忆 / MCP / 情绪）"
        @click="openPanel('memory')"
      >
        ☰
      </button>

      <button
        class="rounded-full px-3 py-1.5 text-sm m3-on-surface-variant hover:bg-zinc-800"
        @click="chat.settingsOpen = true"
      >
        设置
      </button>
      <button
        class="rounded-full p-2 text-sm m3-on-surface-variant hover:bg-zinc-800"
        title="切换主题"
        @click="chat.toggleTheme()"
      >
        {{ chat.theme === 'dark' ? '☀️' : '🌙' }}
      </button>
    </header>

    <!-- 消息列表 -->
    <main ref="scrollContainer" class="flex-1 overflow-y-auto px-3 sm:px-4">
      <MessageList />
    </main>

    <!-- 输入区 -->
    <footer
      class="m3-surface-container-low shrink-0 border-t border-zinc-500 px-3 pt-3 sm:px-4"
      :style="{
        paddingBottom: `calc(${keyboardInset}px + max(1.25rem, env(safe-area-inset-bottom)))`,
      }"
    >
      <MessageInput />
      <!-- 底部状态栏 -->
      <div class="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs m3-on-surface-variant">
        <span class="shrink-0">😊 {{ chat.emotion.top || '平静' }}</span>
        <span class="shrink-0">💾 记忆: {{ chat.memoryCount }} 条</span>
        <span class="hidden shrink-0 sm:inline">🔌 MCP: {{ mcp.servers.filter((s) => s.running).length }} 个运行中</span>
      </div>
    </footer>

    <!-- 侧边面板 -->
    <SidePanel />

    <!-- 设置面板 -->
    <SettingsPanel v-if="chat.settingsOpen" />
  </div>
</template>
