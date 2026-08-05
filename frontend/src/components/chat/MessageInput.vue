<script setup lang="ts">
import { ref } from 'vue'
import { useChatStore } from '../../stores/chat'

const chat = useChatStore()
const text = ref('')

function send() {
  const value = text.value.trim()
  if (!value || chat.sending) return
  chat.sendText(value)
  text.value = ''
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
      class="m3-surface-container-highest flex h-11 w-11 shrink-0 items-center justify-center rounded-full text-lg m3-on-surface-variant transition hover:opacity-80"
      title="语音输入（开发中）"
      disabled
    >
      🎤
    </button>
    <textarea
      v-model="text"
      rows="1"
      placeholder="和 Alice 聊聊..."
      class="m3-surface-container-highest max-h-32 flex-1 resize-none rounded-[20px] border border-zinc-500 px-4 py-3 text-sm m3-on-surface-variant placeholder-zinc-500 focus:border-purple-500 focus:outline-none"
      @keydown="onKeydown"
    />
    <button
      class="m3-primary h-11 shrink-0 rounded-full px-5 text-sm font-medium shadow-lg transition hover:opacity-90 disabled:opacity-40"
      :disabled="!text.trim() || chat.sending"
      @click="send"
    >
      {{ chat.sending ? '回复中...' : '发送' }}
    </button>
  </div>
</template>
