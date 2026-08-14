<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useChatStore } from '../../stores/chat'
import { getEmotionState } from '../../services/api'

const chat = useChatStore()

// 打开面板时主动拉取当前情绪（不必等下一次对话）
onMounted(async () => {
  if (Object.keys(chat.emotion.state).length === 0) {
    try {
      const s = await getEmotionState()
      chat.emotion = { top: s.top ?? '', state: s.state ?? {} }
    } catch {
      // 后端不可用时保持现状
    }
  }
})

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

// 主导情绪的颜色语义（按维度映射到 M3 色）
function barClass(name: string): string {
  switch (name) {
    case '开心':
      return 'bg-[var(--md-sys-color-tertiary)]'
    case '失落':
      return 'bg-[var(--md-sys-color-primary)]'
    case '温柔':
      return 'bg-[var(--md-sys-color-secondary)]'
    case '焦虑':
      return 'bg-[var(--md-sys-color-error)]'
    default:
      return 'bg-[var(--md-sys-color-primary)]'
  }
}
</script>

<template>
  <div class="flex flex-col gap-5">
    <!-- 当前主导情绪 -->
    <div class="m3-card m3-card--filled flex items-center gap-4">
      <div
        class="m3-avatar m3-avatar--alice flex h-14 w-14 items-center justify-center text-2xl"
      >
        {{ topEmotion?.slice(0, 1) ?? '–' }}
      </div>
      <div class="min-w-0">
        <div class="m3-label-medium m3-on-surface-variant">当前主导情绪</div>
        <div v-if="topEmotion" class="m3-title-large mt-0.5 truncate">{{ topEmotion }}</div>
        <div v-else class="m3-body-medium m3-on-surface-variant mt-0.5">暂无数据，说句话试试</div>
      </div>
    </div>

    <!-- 情绪维度（M3 线性进度） -->
    <div class="flex flex-col gap-4">
      <div v-for="d in dims" :key="d.name" class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <span class="m3-label-medium m3-on-surface">{{ d.name }}</span>
          <span class="m3-label-small m3-on-surface-variant tabular-nums">
            {{ (d.value * 100).toFixed(0) }}
          </span>
        </div>
        <div class="m3-linear-progress">
          <div
            class="m3-linear-progress__bar"
            :class="barClass(d.name)"
            :style="{ transform: `scaleX(${Math.min(1, d.value)})` }"
          />
        </div>
      </div>
    </div>

    <!-- 主动推送开关 -->
    <div class="m3-card m3-card--outlined flex items-center justify-between gap-3">
      <div class="min-w-0">
        <div class="m3-title-small">主动推送</div>
        <div class="m3-body-small m3-on-surface-variant mt-0.5">情绪积累时 Alice 主动找你说话</div>
      </div>
      <label class="m3-switch shrink-0">
        <input
          type="checkbox"
          :checked="chat.proactiveEnabled"
          @change="chat.toggleProactive()"
        />
        <span class="m3-switch__track" />
        <span class="m3-switch__thumb" />
      </label>
    </div>

    <p class="m3-body-small m3-on-surface-variant leading-relaxed">
      情绪由对话驱动、随时间自然衰减。情绪积累到一定程度，Alice 会主动来找你说话。
    </p>
  </div>
</template>
