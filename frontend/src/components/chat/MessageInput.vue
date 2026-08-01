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
      class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-lg text-zinc-400 hover:bg-zinc-800"
      title="语音输入（阶段二上线）"
      disabled
    >
      🎤
    </button>
    <textarea
      v-model="text"
      rows="1"
      placeholder="和 Alice 聊聊..."
      class="max-h-32 flex-1 resize-none rounded-xl border border-zinc-700 bg-zinc-900 px-4 py-2.5 text-sm text-zinc-100 placeholder-zinc-500 focus:border-purple-500 focus:outline-none"
      @keydown="onKeydown"
    />
    <button
      class="h-10 shrink-0 rounded-xl bg-purple-600 px-5 text-sm font-medium text-white transition hover:bg-purple-500 disabled:opacity-40"
      :disabled="!text.trim() || chat.sending"
      @click="send"
    >
      {{ chat.sending ? '回复中...' : '发送' }}
    </button>
  </div>
</template>
