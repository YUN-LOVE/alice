<script setup lang="ts">
import { computed } from 'vue'
import { useChatStore } from '../../stores/chat'

const chat = useChatStore()
// 情绪维度 + 中文名
const dims = computed(() => {
  const state = chat.emotion.state
  const order = ['开心', '失落', '温柔', '焦虑', '好奇']
  const list = order
    .filter((d) => d in state)
    .map((d) => ({ name: d, value: state[d] ?? 0 }))
  // 未在预置顺序里的维度（未来扩展）追加
  for (const [k, v] of Object.entries(state)) {
    if (!order.includes(k)) list.push({ name: k, value: v })
  }
  return list
})

const topEmotion = computed(() => chat.emotion.top)
</script>

<template>
  <div class="flex flex-col gap-4">
    <div class="text-xs text-zinc-500">当前主导情绪：</div>
    <div v-if="topEmotion" class="text-2xl font-medium text-zinc-100">{{ topEmotion }}</div>
    <div v-else class="text-sm text-zinc-600">暂无情绪数据，说句话试试</div>

    <div class="flex flex-col gap-3">
      <div v-for="d in dims" :key="d.name" class="flex items-center gap-3">
        <span class="w-10 shrink-0 text-sm text-zinc-300">{{ d.name }}</span>
        <div class="h-2.5 flex-1 overflow-hidden rounded-full bg-zinc-800">
          <div
            class="h-full rounded-full bg-gradient-to-r from-purple-500 to-pink-500 transition-all duration-700"
            :style="{ width: `${Math.round(d.value * 100)}%` }"
          />
        </div>
        <span class="w-9 shrink-0 text-right text-xs text-zinc-500">
          {{ (d.value * 100).toFixed(0) }}
        </span>
      </div>
    </div>

    <p class="mt-2 border-t border-zinc-800 pt-3 text-xs leading-relaxed text-zinc-500">
      情绪由对话驱动、随时间自然衰减。情绪积累到一定程度，Alice 会主动来找你说话。
    </p>

    <!-- 主动推送开关 -->
    <div class="mt-2 flex items-center justify-between border-t border-zinc-800 pt-3">
      <div>
        <div class="text-sm text-zinc-200">主动推送</div>
        <div class="text-xs text-zinc-500">情绪积累时 Alice 主动找你说话</div>
      </div>
      <button
        class="relative h-5 w-9 rounded-full transition"
        :class="chat.proactiveEnabled ? 'bg-purple-500' : 'bg-zinc-700'"
        role="switch"
        :aria-checked="chat.proactiveEnabled"
        @click="chat.toggleProactive()"
      >
        <span
          class="absolute top-0.5 h-4 w-4 rounded-full bg-white transition"
          :class="chat.proactiveEnabled ? 'left-[18px]' : 'left-0.5'"
        />
      </button>
    </div>
  </div>
</template>
