<script setup lang="ts">
import { useChatStore } from '../../stores/chat'
import { renderMarkdown } from '../../services/markdown'
import { httpBase } from '../../services/backend'

const chat = useChatStore()

function formatTime(ts?: number): string {
  if (!ts) return ''
  const d = new Date(ts)
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  return `${hh}:${mm}`
}

// 提取消息中的图片路径（[图片:/uploads/files/xxx.png]）
function extractImages(content: string): string[] {
  const out: string[] = []
  const re = /\[图片:([^\]]+)\]/g
  let m: RegExpExecArray | null
  while ((m = re.exec(content))) out.push(m[1])
  return out
}

// 渲染时把图片标记替换为占位文本（图片单独预览）
function displayContent(content: string): string {
  return content.replace(/\[图片:[^\]]+\]/g, '图片')
}
</script>

<template>
  <div class="mx-auto flex w-full max-w-3xl flex-col gap-5 px-3 py-5 sm:px-5">
    <!-- 空状态 -->
    <div v-if="chat.messages.length === 0" class="mt-[24vh] flex flex-col items-center text-center">
      <div
        class="m3-avatar m3-avatar--alice m3-ripple flex h-20 w-20 items-center justify-center text-3xl shadow-lg"
      >
        A
      </div>
      <p class="m3-title-large m3-on-surface mt-5">你好，我是 Alice</p>
      <p class="m3-body-medium m3-on-surface-variant mt-2 max-w-xs">
        我们聊聊吧，不用记住我什么——我都记得。
      </p>
    </div>

    <!-- 消息 -->
    <div
      v-for="m in chat.messages"
      :key="m.id"
      class="m3-fade-enter-active m3-slide-up-enter-active flex items-end gap-2"
      :class="m.role === 'user' ? 'justify-end' : 'justify-start'"
    >
      <!-- Alice 头像 -->
      <div
        v-if="m.role === 'assistant'"
        class="m3-avatar m3-avatar--alice mb-1 h-8 w-8 shrink-0 text-sm"
      >
        A
      </div>

      <div
        class="flex max-w-[86%] flex-col md:max-w-[76%]"
        :class="m.role === 'user' ? 'items-end' : 'items-start'"
      >
        <!-- 图片预览（附件） -->
        <template v-if="m.role === 'user'">
          <img
            v-for="img in extractImages(m.content)"
            :key="img"
            :src="httpBase() + img"
            class="mb-1.5 block max-h-56 rounded-[20px] border border-[var(--md-sys-color-outline-variant)] object-cover"
            loading="lazy"
            alt="图片"
          />
        </template>

        <!-- 气泡 -->
        <div
          class="m3-ripple px-4 py-2.5 text-[15px] leading-relaxed shadow-sm"
          :class="
            m.role === 'user'
              ? 'm3-primary-container-bg rounded-[20px] rounded-br-[4px]'
              : 'm3-surface-container-high rounded-[20px] rounded-bl-[4px]'
          "
        >
          <!-- 流式中的消息：纯文本保持流畅；完成后渲染 Markdown -->
          <template v-if="m.streaming">
            <span class="whitespace-pre-wrap">{{ m.content }}</span>
            <span v-if="!m.content" class="m3-on-surface-variant opacity-60">正在输入...</span>
            <span
              v-if="m.content"
              class="ml-0.5 inline-block h-4 w-0.5 animate-pulse bg-[var(--md-sys-color-primary)] align-middle"
            />
          </template>
          <div
            v-else
            class="md-content"
            v-html="renderMarkdown(displayContent(m.content)).trim()"
          />
        </div>

        <!-- 时间戳 -->
        <span
          v-if="m.time"
          class="m3-label-small m3-on-surface-variant mt-1 px-1 leading-none"
        >{{ formatTime(m.time) }}</span>
      </div>
    </div>
  </div>
</template>
