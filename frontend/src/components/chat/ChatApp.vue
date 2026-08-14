<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useChatStore } from '../../stores/chat'
import { useMCPStore } from '../../stores/mcp'
import MessageList from './MessageList.vue'
import MessageInput from './MessageInput.vue'
import AudioPlayer from './AudioPlayer.vue'
import SettingsPanel from '../settings/SettingsPanel.vue'
import SidePanel from '../panel/SidePanel.vue'

const chat = useChatStore()
const mcp = useMCPStore()
const scrollContainer = ref<HTMLElement | null>(null)
const scrolled = ref(false)
const keyboardInset = ref(0) // 虚拟键盘遮挡高度（旧浏览器兼容，键盘弹出时把输入区顶起）
let unwatch: (() => void) | null = null

async function scrollToBottom() {
  await nextTick()
  if (scrollContainer.value) {
    scrollContainer.value.scrollTo({ top: scrollContainer.value.scrollHeight })
  }
}

function onScroll() {
  const el = scrollContainer.value
  scrolled.value = !!el && el.scrollTop > 4
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
  mcp.registerCapabilities() // 监听 MCP 能力变化广播（其他端操作实时同步）
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
    <!-- ===== Top App Bar（M3 Small） ===== -->
    <header class="m3-topbar shrink-0 border-b border-[var(--md-sys-color-outline-variant)]" :class="{ 'm3-topbar--scrolled': scrolled }">
      <div
        class="m3-avatar m3-avatar--alice m3-ripple h-10 w-10 select-none text-base"
        title="Alice"
      >
        A
      </div>
      <div class="m3-topbar__title">
        <h1>Alice</h1>
        <p class="flex items-center gap-1.5">
          <span
            class="inline-block h-2 w-2 shrink-0 rounded-full"
            :class="chat.connected ? 'bg-[var(--md-sys-color-tertiary)]' : 'bg-[var(--md-sys-color-error)]'"
          />
          <span class="truncate">{{ chat.connected ? (chat.serverInfo?.llm ?? '连接中...') : '未连接' }}</span>
        </p>
      </div>

      <!-- 桌面端快捷入口 -->
      <nav class="hidden shrink-0 items-center gap-1 md:flex">
        <button class="m3-btn m3-btn--text m3-state-layer m3-ripple" @click="openPanel('memory')">
          <span class="m3-icon m3-icon--sm">psychology</span>记忆
        </button>
        <button class="m3-btn m3-btn--text m3-state-layer m3-ripple" @click="openPanel('mcp')">
          <span class="m3-icon m3-icon--sm">extension</span>MCP
        </button>
        <button class="m3-btn m3-btn--text m3-state-layer m3-ripple" @click="openPanel('emotion')">
          <span class="m3-icon m3-icon--sm">sentiment_satisfied</span>情绪
        </button>
      </nav>

      <!-- 移动端：打开抽屉 -->
      <button
        class="m3-icon-btn m3-state-layer m3-ripple md:hidden"
        title="菜单（记忆 / MCP / 情绪）"
        @click="openPanel('memory')"
      >
        <span class="m3-icon">menu</span>
      </button>

      <button
        class="m3-icon-btn m3-state-layer m3-ripple"
        :title="chat.theme === 'dark' ? '切换为浅色' : '切换为深色'"
        @click="chat.toggleTheme()"
      >
        <span class="m3-icon">{{ chat.theme === 'dark' ? 'dark_mode' : 'light_mode' }}</span>
      </button>
      <button
        class="m3-icon-btn m3-state-layer m3-ripple"
        title="设置"
        @click="chat.settingsOpen = true"
      >
        <span class="m3-icon">settings</span>
      </button>
    </header>

    <!-- ===== 消息列表 ===== -->
    <main
      ref="scrollContainer"
      class="flex-1 overflow-y-auto overscroll-contain"
      @scroll="onScroll"
    >
      <MessageList />
    </main>

    <!-- ===== 输入区 ===== -->
    <footer
      class="shrink-0 border-t border-[var(--md-sys-color-outline-variant)] px-3 pt-3 sm:px-4"
      :style="{
        paddingBottom: `calc(${keyboardInset}px + max(1rem, env(safe-area-inset-bottom)))`,
      }"
    >
      <MessageInput />
      <!-- 底部状态行（M3：label-medium + on-surface-variant） -->
      <div class="m3-label-medium m3-on-surface-variant mx-1 mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 pb-1">
        <span class="inline-flex items-center gap-1">
          <span class="m3-icon m3-icon--xs">sentiment_satisfied</span>
          <span>{{ chat.emotion.top || '平静' }}</span>
        </span>
        <span class="inline-flex items-center gap-1">
          <span class="m3-icon m3-icon--xs">database</span>
          <span>{{ chat.memoryCount }} 条记忆</span>
        </span>
        <span class="hidden items-center gap-1 sm:inline-flex">
          <span class="m3-icon m3-icon--xs">extension</span>
          <span>{{ mcp.servers.filter((s) => s.running).length }} 个 MCP 运行中</span>
        </span>
      </div>
    </footer>

    <!-- ===== 侧边抽屉（M3 Navigation Drawer） ===== -->
    <SidePanel />

    <!-- ===== 设置对话框 ===== -->
    <SettingsPanel v-if="chat.settingsOpen" />

    <!-- ===== 回复语音播放条 ===== -->
    <AudioPlayer />
  </div>
</template>
