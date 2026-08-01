<script setup lang="ts">
import { ref, nextTick } from 'vue'
import { useChatStore } from '../../stores/chat'

const chat = useChatStore()

// 消息列表 DOM 滚动到底（由 ChatApp 的 $subscribe 触发 nextTick 后执行）
const listRef = ref<HTMLElement | null>(null)
</script>

<template>
  <div ref="listRef" class="flex flex-col gap-4 py-4">
    <template v-if="chat.messages.length === 0">
      <div class="mt-16 text-center text-zinc-500">
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
        class="mr-2 flex h-7 w-7 shrink-0 items-center justify-center self-end rounded-full bg-gradient-to-br from-purple-500 to-pink-500 text-xs font-bold"
      >
        A
      </div>
      <div
        class="max-w-[75%] whitespace-pre-wrap rounded-2xl px-4 py-2 text-sm leading-relaxed"
        :class="
          m.role === 'user'
            ? 'rounded-br-sm bg-purple-600 text-white'
            : 'rounded-bl-sm bg-zinc-800 text-zinc-100'
        "
      >
        {{ m.content }}
        <span v-if="m.streaming && !m.content" class="text-zinc-400">正在输入...</span>
        <span
          v-if="m.streaming && m.content"
          class="ml-0.5 inline-block h-4 w-0.5 animate-pulse bg-zinc-400 align-middle"
        />
      </div>
    </div>
  </div>
</template>
