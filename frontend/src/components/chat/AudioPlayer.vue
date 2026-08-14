<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'
import { useChatStore } from '../../stores/chat'

const chat = useChatStore()
const audio = ref<HTMLAudioElement | null>(null)
const playing = ref(false)
const current = ref(0)
const duration = ref(0)

function fmt(sec: number): string {
  if (!Number.isFinite(sec) || sec <= 0) return '0:00'
  const m = Math.floor(sec / 60)
  const s = Math.floor(sec % 60)
  return `${m}:${String(s).padStart(2, '0')}`
}

function togglePlay() {
  const el = audio.value
  if (!el) return
  if (playing.value) {
    el.pause()
  } else {
    void el.play().catch(() => {})
  }
}

function onTimeUpdate() {
  const el = audio.value
  if (!el) return
  current.value = el.currentTime
  duration.value = el.duration || 0
}

function seek(e: Event) {
  const el = audio.value
  if (!el) return
  const v = Number((e.target as HTMLInputElement).value)
  el.currentTime = v
  current.value = v
}

// 新语音到来：自动尝试播放（可能被浏览器自动播放策略拦截，拦截后手动点）
watch(
  () => chat.audio?.url,
  (url, old) => {
    if (url && url !== old) {
      current.value = 0
      duration.value = 0
      playing.value = false
      const el = audio.value
      if (el) {
        void el.play().catch(() => {})
      }
    }
  },
)

onUnmounted(() => {
  audio.value?.pause()
})
</script>

<template>
  <Transition name="m3-slide-up">
    <div v-if="chat.audio" class="fixed bottom-28 left-1/2 z-40 w-[min(92vw,520px)] -translate-x-1/2">
      <div
        class="m3-surface-container-high flex items-center gap-2 rounded-[28px] border border-[var(--md-sys-color-outline-variant)] py-2 pl-2 pr-3 shadow-[var(--md-elevation-3)]"
      >
        <audio
          ref="audio"
          :src="chat.audio.url"
          @play="playing = true"
          @pause="playing = false"
          @timeupdate="onTimeUpdate"
          @ended="playing = false"
        />
        <button
          class="m3-icon-btn m3-icon-btn--filled m3-state-layer m3-ripple"
          :title="playing ? '暂停' : '播放'"
          @click="togglePlay"
        >
          <span class="m3-icon">{{ playing ? 'pause' : 'play_arrow' }}</span>
        </button>
        <div class="flex min-w-0 flex-1 flex-col gap-0.5">
          <span class="m3-label-small m3-on-surface-variant truncate">
            Alice 的语音{{ chat.audio.text ? ` · ${chat.audio.text.slice(0, 24)}${chat.audio.text.length > 24 ? '…' : ''}` : '' }}
          </span>
          <input
            type="range"
            class="m3-slider h-1"
            :max="duration || 0"
            :value="current"
            step="0.1"
            :disabled="!duration"
            @input="seek"
          />
        </div>
        <span class="m3-label-small m3-on-surface-variant shrink-0 tabular-nums">
          {{ fmt(current) }}/{{ fmt(duration) }}
        </span>
        <button
          class="m3-icon-btn m3-icon-btn--xs m3-state-layer m3-ripple"
          title="关闭"
          @click="chat.dismissAudio()"
        >
          <span class="m3-icon m3-icon--sm">close</span>
        </button>
      </div>
    </div>
  </Transition>
</template>
