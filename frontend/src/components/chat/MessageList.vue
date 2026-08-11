<script setup lang="ts">
import { useChatStore } from '../../stores/chat'
import { renderMarkdown } from '../../services/markdown'

const chat = useChatStore()

function formatTime(ts?: number): string {
  if (!ts) return ''
  const d = new Date(ts)
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  return `${hh}:${mm}`
}
</script>

<template>
  <div class="flex flex-col gap-4 py-4">
    <template v-if="chat.messages.length === 0">
      <div class="mt-[30vh] text-center text-zinc-500">
        <p class="text-lg">你好，我是 Alice</p>
        <p class="mt-1 text-sm">我们聊聊吧，不用记住我什么——我都记得。</p>
      </div>
    </template>

    <div
      v-for="m in chat.messages"
      :key="m.id"
      class="flex"
      :class="m.role === 'user' ? 'justify-end' : 'justify-start'"
    >
      <div
        v-if="m.role === 'assistant'"
        class="mr-2 flex h-8 w-8 shrink-0 items-center justify-center self-end rounded-full bg-gradient-to-br from-purple-500 to-pink-500 text-sm font-bold m3-on-primary"
      >
        A
      </div>
      <div
        class="flex max-w-[88%] flex-col md:max-w-[75%]"
        :class="m.role === 'user' ? 'items-end' : 'items-start'"
      >
        <div
          class="px-4 py-2.5 text-sm leading-relaxed"
          :class="
            m.role === 'user'
              ? 'm3-primary rounded-[24px] rounded-br-[6px]'
              : 'm3-surface-container-highest rounded-[24px] rounded-bl-[6px]'
          "
        >
          <!-- 流式中的消息：纯文本保持流畅；完成后渲染 Markdown -->
          <template v-if="m.streaming">
            <span class="whitespace-pre-wrap">{{ m.content }}</span>
            <span v-if="!m.content" class="opacity-50">正在输入...</span>
            <span
              v-if="m.content"
              class="ml-0.5 inline-block h-4 w-0.5 animate-pulse bg-zinc-400 align-middle"
            />
          </template>
          <div
            v-else
            class="md-content whitespace-pre-wrap"
            v-html="renderMarkdown(m.content)"
          />
        </div>
        <span
          v-if="m.time"
          class="mt-0.5 px-1 text-[10px] leading-none m3-on-surface-variant"
        >{{ formatTime(m.time) }}</span>
      </div>
    </div>
  </div>
</template>
