<script setup lang="ts">
import { ref } from 'vue'
import { useChatStore } from '../../stores/chat'

const chat = useChatStore()
const text = ref('')
const ta = ref<HTMLTextAreaElement | null>(null)

function autoGrow() {
  const el = ta.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 160) + 'px'
}

function send() {
  const value = text.value.trim()
  if (!value || chat.sending) return
  chat.sendText(value)
  text.value = ''
  requestAnimationFrame(() => {
    if (ta.value) ta.value.style.height = ''
    ta.value?.focus()
  })
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    send()
  }
}
</script>

<template>
  <div class="flex items-end gap-2">
    <button
      class="m3-surface-container-highest flex h-12 w-12 shrink-0 items-center justify-center rounded-full text-lg m3-on-surface-variant transition hover:opacity-80"
      title="语音输入（开发中）"
      disabled
    >
      🎤
    </button>
    <textarea
      ref="ta"
      v-model="text"
      rows="1"
      placeholder="和 Alice 聊聊..."
      class="m3-surface-container-highest min-h-[52px] flex-1 resize-none rounded-[24px] border border-zinc-500 px-4 py-3.5 text-[15px] leading-relaxed m3-on-surface-variant placeholder-zinc-500 focus:border-purple-500 focus:outline-none"
      @input="autoGrow"
      @keydown="onKeydown"
    />
    <button
      class="m3-primary h-12 shrink-0 rounded-full px-5 text-sm font-medium shadow-lg transition hover:opacity-90 disabled:opacity-40"
      :disabled="!text.trim() || chat.sending"
      @click="send"
    >
      {{ chat.sending ? '回复中...' : '发送' }}
    </button>
  </div>
</template>
