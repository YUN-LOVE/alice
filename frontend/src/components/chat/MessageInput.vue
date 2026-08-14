<script setup lang="ts">
import { ref } from 'vue'
import { useChatStore } from '../../stores/chat'
import { VoiceRecorder, sttUpload } from '../../services/audio'
import { uploadFile } from '../../services/upload'
import { httpBase } from '../../services/backend'

const chat = useChatStore()
const text = ref('')
const ta = ref<HTMLTextAreaElement | null>(null)

// ============ 语音输入 ============
const recorder = new VoiceRecorder()
const recording = ref(false)
const transcribing = ref(false)
const recordSec = ref(0)
const micError = ref('')

async function toggleVoice() {
  if (!VoiceRecorder.supported) {
    micError.value = '当前浏览器不支持录音'
    return
  }
  if (!recording.value) {
    micError.value = ''
    try {
      recorder.onTick = (s) => (recordSec.value = Math.floor(s))
      await recorder.start()
      recording.value = true
      recordSec.value = 0
    } catch (e: any) {
      micError.value = e?.name === 'NotAllowedError' ? '麦克风权限被拒绝' : `无法开始录音: ${e?.message ?? e}`
    }
    return
  }
  // 停止录音 → 转写
  recording.value = false
  transcribing.value = true
  try {
    const blob = await recorder.stop()
    const recognized = await sttUpload(blob)
    if (recognized) {
      text.value = text.value ? `${text.value} ${recognized}` : recognized
      autoGrow()
    }
  } catch (e: any) {
    micError.value = e?.message ?? '语音识别失败'
  } finally {
    transcribing.value = false
  }
}

function cancelVoice() {
  recorder.cancel()
  recording.value = false
  transcribing.value = false
  recordSec.value = 0
}

// ============ 图片上传 ============
const fileInput = ref<HTMLInputElement | null>(null)
const attaching = ref(false)
const attachProgress = ref(0)
const attachPath = ref('') // 上传完成后的后端路径

async function onPickImage(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  attaching.value = true
  attachProgress.value = 0
  try {
    const res = await uploadFile(file, (pct) => (attachProgress.value = pct))
    attachPath.value = res.path
  } catch (err: any) {
    micError.value = err?.message ?? '图片上传失败'
  } finally {
    attaching.value = false
    if (fileInput.value) fileInput.value.value = ''
  }
}

function removeAttach() {
  attachPath.value = ''
}

// ============ 发送 ============
function autoGrow() {
  const el = ta.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 160) + 'px'
}

function send() {
  let value = text.value.trim()
  if (attachPath.value) {
    value = value ? `${value}\n[图片:${attachPath.value}]` : `[图片:${attachPath.value}]`
  }
  if (!value || chat.sending) return
  chat.sendText(value)
  text.value = ''
  attachPath.value = ''
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
  <div class="mx-auto flex w-full max-w-3xl flex-col gap-1.5">
    <!-- 录音状态行 -->
    <div
      v-if="recording || transcribing"
      class="m3-fade-enter-active flex items-center gap-2 px-2"
    >
      <span v-if="recording" class="m3-label-medium inline-flex items-center gap-2 text-[var(--md-sys-color-error)]">
        <span class="inline-block h-2.5 w-2.5 animate-pulse rounded-full bg-[var(--md-sys-color-error)]" />
        录音中 {{ recordSec }}s
      </span>
      <span v-else class="m3-label-medium m3-on-surface-variant inline-flex items-center gap-2">
        <span class="m3-spinner h-3.5 w-3.5" />
        识别中...
      </span>
      <span class="flex-1" />
      <button class="m3-btn m3-btn--text m3-state-layer m3-ripple m3-btn--sm" @click="cancelVoice">
        取消
      </button>
      <button v-if="recording" class="m3-btn m3-btn--tonal m3-state-layer m3-ripple m3-btn--sm" @click="toggleVoice">
        完成
      </button>
    </div>

    <!-- 图片附件预览 -->
    <div v-if="attachPath" class="m3-fade-enter-active flex items-center gap-2 px-1">
      <img
        :src="httpBase() + attachPath"
        class="h-14 w-14 rounded-[12px] border border-[var(--md-sys-color-outline-variant)] object-cover"
      />
      <span class="m3-label-small m3-on-surface-variant truncate">{{ attachPath.split('/').pop() }}</span>
      <span class="flex-1" />
      <button
        class="m3-icon-btn m3-icon-btn--sm m3-state-layer m3-ripple"
        title="移除图片"
        @click="removeAttach"
      >
        <span class="m3-icon m3-icon--sm">close</span>
      </button>
    </div>
    <div v-else-if="attaching" class="flex items-center gap-2 px-2">
      <span class="m3-label-medium m3-on-surface-variant">上传中 {{ attachProgress }}%</span>
      <div class="m3-linear-progress w-28">
        <div class="m3-linear-progress__bar" :style="{ transform: `scaleX(${attachProgress / 100})` }" />
      </div>
    </div>

    <p v-if="micError" class="m3-label-small px-2 text-[var(--md-sys-color-error)]">{{ micError }}</p>

    <!-- 输入行：M3 填充式圆角输入框 -->
    <div class="flex items-end gap-2">
      <div class="m3-field__box m3-field__box--rounded m3-surface-container-high flex-1">
        <button
          class="m3-icon-btn m3-state-layer m3-ripple shrink-0"
          :class="recording ? 'm3-icon-btn--danger' : ''"
          :title="recording ? '完成并转文字' : '语音输入'"
          :disabled="transcribing || chat.sending"
          @click="toggleVoice"
        >
          <span class="m3-icon">{{ transcribing ? 'sync' : 'mic' }}</span>
        </button>
        <textarea
          ref="ta"
          v-model="text"
          rows="1"
          placeholder="和 Alice 聊聊..."
          class="max-h-40 min-h-[48px] flex-1 resize-none bg-transparent px-1 py-3.5 text-[15px] leading-relaxed text-[var(--md-sys-color-on-surface)] placeholder-[var(--md-sys-color-on-surface-variant)] outline-none"
          @input="autoGrow"
          @keydown="onKeydown"
        />
        <button
          class="m3-icon-btn m3-state-layer m3-ripple shrink-0"
          title="发送图片（Alice 可以看图片）"
          :disabled="attaching || chat.sending"
          @click="fileInput?.click()"
        >
          <span class="m3-icon">image</span>
        </button>
        <input ref="fileInput" type="file" accept="image/*" class="hidden" @change="onPickImage" />
      </div>

      <button
        class="m3-btn m3-btn--filled m3-state-layer m3-ripple h-12 min-w-[88px]"
        :disabled="(!text.trim() && !attachPath) || chat.sending"
        @click="send"
      >
        <span v-if="chat.sending" class="m3-spinner h-4 w-4 border-2" />
        {{ chat.sending ? '回复中' : '发送' }}
      </button>
    </div>
  </div>
</template>
