<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useChatStore } from '../../stores/chat'
import { THEME_STYLES } from '../../styles/theme'
import { ws } from '../../services/ws'
import {
  getSettings,
  getVoices,
  ttsPreview,
  type RuntimeSettings,
  type VoiceInfo,
} from '../../services/api'
import { httpBase } from '../../services/backend'

const chat = useChatStore()
const tab = ref<'appearance' | 'dialog'>('appearance')
const url = ref(chat.backendUrl)
const saved = ref(false)
const connecting = ref(false)
const wallpaperInfo = ref('')

// Material You 预设种子色（蓝色系默认优先）
const presets = [
  '#00639a', // 蓝（默认）
  '#006c5c', // 绿
  '#7c5cff', // 动态紫
  '#6750a4', // 经典紫
  '#c0004b', // 玫红
  '#d24c00', // 橙
  '#6a9b00', // 黄绿
  '#9a3400', // 赭石
  '#3f3f3f', // 中性
  '#7c6b00', // 橄榄
]

function onWallpaperFile(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  wallpaperInfo.value = '正在取色...'
  const reader = new FileReader()
  reader.onload = async () => {
    const img = new Image()
    img.onload = async () => {
      try {
        const seed = await chat.applyWallpaper(img)
        wallpaperInfo.value = `已从壁纸取色：#${seed.slice(1)}`
      } catch {
        wallpaperInfo.value = '取色失败，请换一张图片'
      }
    }
    img.src = reader.result as string
  }
  reader.readAsDataURL(file)
}

async function save() {
  saved.value = false
  connecting.value = true
  chat.reconnectTo(url.value.trim())
  const timeout = setTimeout(() => finish(), 4000)
  const off = ws.on('handshake_ack', () => {
    clearTimeout(timeout)
    finish()
  })
  const offErr = ws.on('error', () => {
    clearTimeout(timeout)
    finish()
  })
  function finish() {
    off()
    offErr()
    clearTimeout(timeout)
    connecting.value = false
    saved.value = true
    setTimeout(() => (chat.settingsOpen = false), 600)
  }
}

function useDefault() {
  url.value = ''
  save()
}

// ============ 对话设置（LLM / 情绪 / 记忆容量） ============
const settingsLoaded = ref(false)
const llmForm = ref({
  provider: 'deepseek',
  base_url: '',
  api_key: '',
  api_key_configured: false,
  model: '',
  temperature: 0.8,
  max_tokens: 2048,
})
const emotionForm = ref({
  decay_rate: 0.0008,
  max_value: 1,
  threshold: 0.7,
  cooldown_seconds: 600,
  tick_seconds: 1,
  silent_after_minutes: 30,
  skip_if_active_minutes: 10,
  hours_start: 8,
  hours_end: 23,
})
const blockForm = ref({ max_entries: 200 })
const settingsMsg = ref('')
const settingsErr = ref('')
const savingSettings = ref(false)

// ============ 语音设置（Edge TTS 音色/语速/音高/音量） ============
const audioForm = ref({ voice: '', rate: 0, pitch: 0, volume: 0 })
const voices = ref<VoiceInfo[]>([])
const voiceLoading = ref(false)
const previewing = ref(false)
const previewMsg = ref('')
const previewErr = ref('')
const previewAudio = ref<HTMLAudioElement | null>(null)
const savingAudio = ref(false)
const audioMsg = ref('')
const audioErr = ref('')

// 解析 "+10%" → 10 / "+5Hz" → 5
function parseSigned(s: string | undefined): number {
  if (!s) return 0
  const m = String(s).match(/([+-]?\d+(?:\.\d+)?)/)
  return m ? Number(m[1]) : 0
}

// 按语言分组（zh 组优先）
const voiceGroups = computed(() => {
  const groups = new Map<string, VoiceInfo[]>()
  for (const v of voices.value) {
    const key = v.locale.startsWith('zh') ? '中文' : v.locale
    if (!groups.has(key)) groups.set(key, [])
    groups.get(key)!.push(v)
  }
  return [...groups.entries()]
})

async function loadVoices() {
  voiceLoading.value = true
  try {
    const data = await getVoices('zh')
    voices.value = data.voices
  } catch {
    previewErr.value = '获取音色列表失败（TTS Server 未运行？）'
  } finally {
    voiceLoading.value = false
  }
}

async function previewTTS() {
  previewing.value = true
  previewMsg.value = ''
  previewErr.value = ''
  try {
    const res = await ttsPreview({
      text: '你好，我是 Alice，这是语音设置试听。',
      voice: audioForm.value.voice,
      rate: fmtSigned(audioForm.value.rate, '%'),
      pitch: fmtSigned(audioForm.value.pitch, 'Hz'),
      volume: fmtSigned(audioForm.value.volume, '%'),
    })
    previewMsg.value = '试听中...'
    const el = previewAudio.value
    if (el) {
      el.src = httpBase() + res.url
      void el.play().catch(() => {
        previewErr.value = '浏览器阻止了自动播放，请点击播放'
      })
    }
  } catch (e: any) {
    previewErr.value = e?.message ?? '试听失败'
  } finally {
    previewing.value = false
  }
}

function fmtSigned(n: number, unit: string): string {
  if (n === 0) return `+0${unit}`
  return n > 0 ? `+${n}${unit}` : `${n}${unit}`
}

function saveAudio() {
  savingAudio.value = true
  audioMsg.value = ''
  audioErr.value = ''
  ws.send('settings_update', {
    section: 'audio',
    values: {
      tts_voice: audioForm.value.voice,
      tts_rate: fmtSigned(audioForm.value.rate, '%'),
      tts_pitch: fmtSigned(audioForm.value.pitch, 'Hz'),
      tts_volume: fmtSigned(audioForm.value.volume, '%'),
    },
  })
  const off = ws.on('settings_update_ack', (p: any) => {
    if (p?.section !== 'audio') return
    off()
    savingAudio.value = false
    if (p.ok) {
      audioMsg.value = '已保存，语音参数热重载生效'
      setTimeout(() => (audioMsg.value = ''), 3000)
    } else {
      audioErr.value = p.message ?? '保存失败'
    }
  })
  setTimeout(() => {
    off()
    savingAudio.value = false
    if (!audioMsg.value) audioErr.value = '保存超时（后端未确认）'
  }, 4000)
}

const providers = ['deepseek', 'openai', 'siliconflow', 'mock']

onMounted(async () => {
  try {
    const s: RuntimeSettings = await getSettings()
    llmForm.value = { ...s.llm, api_key: '', api_key_configured: s.llm.api_key_configured }
    emotionForm.value = {
      ...s.emotion,
      hours_start: s.emotion.hours?.[0] ?? 8,
      hours_end: s.emotion.hours?.[1] ?? 23,
    }
    blockForm.value = { max_entries: s.block.max_entries }
    audioForm.value = {
      voice: s.audio?.tts_voice ?? '',
      rate: parseSigned(s.audio?.tts_rate),
      pitch: parseSigned(s.audio?.tts_pitch),
      volume: parseSigned(s.audio?.tts_volume),
    }
    settingsLoaded.value = true
    void loadVoices()
  } catch {
    settingsErr.value = '无法加载当前设置（后端不可用？）'
  }
})

function applySettings() {
  if (!settingsLoaded.value) return
  savingSettings.value = true
  settingsMsg.value = ''
  settingsErr.value = ''

  const llmV: Record<string, any> = {
    provider: llmForm.value.provider,
    base_url: llmForm.value.base_url.trim(),
    model: llmForm.value.model.trim(),
    temperature: Number(llmForm.value.temperature),
    max_tokens: Number(llmForm.value.max_tokens),
  }
  const apiKey = llmForm.value.api_key.trim()
  if (apiKey) llmV.api_key = apiKey
  ws.send('settings_update', { section: 'llm', values: llmV })

  ws.send('settings_update', {
    section: 'emotion',
    values: {
      decay_rate: Number(emotionForm.value.decay_rate),
      max_value: Number(emotionForm.value.max_value),
      threshold: Number(emotionForm.value.threshold),
      cooldown_seconds: Number(emotionForm.value.cooldown_seconds),
      tick_seconds: Number(emotionForm.value.tick_seconds),
      silent_after_minutes: Number(emotionForm.value.silent_after_minutes),
      skip_if_active_minutes: Number(emotionForm.value.skip_if_active_minutes),
      hours: [Number(emotionForm.value.hours_start), Number(emotionForm.value.hours_end)],
    },
  })

  ws.send('settings_update', {
    section: 'block',
    values: { max_entries: Number(blockForm.value.max_entries) },
  })

  const off = ws.on('settings_update_ack', () => {
    off()
    savingSettings.value = false
    settingsMsg.value = '已保存，配置热重载生效'
    setTimeout(() => (settingsMsg.value = ''), 3000)
  })
  setTimeout(() => {
    off()
    savingSettings.value = false
    if (!settingsMsg.value) settingsErr.value = '保存超时（后端未确认）'
  }, 4000)
}
</script>

<template>
  <Teleport to="body">
    <Transition name="m3-fade">
      <div
        v-if="chat.settingsOpen"
        class="m3-dialog-overlay"
        @click.self="chat.settingsOpen = false"
      >
        <div class="m3-dialog m3-dialog--wide m3-dialog-enter" role="dialog" aria-label="设置">
          <!-- 头部 -->
          <div class="m3-dialog__head flex items-center justify-between !pb-2">
            <div class="m3-dialog__title">设置</div>
            <button
              class="m3-icon-btn m3-state-layer m3-ripple"
              title="关闭"
              @click="chat.settingsOpen = false"
            >
              <span class="m3-icon">close</span>
            </button>
          </div>

          <!-- Tabs -->
          <div class="m3-tabs px-4">
            <button
              class="m3-tab m3-state-layer m3-ripple"
              :class="{ 'm3-tab--active': tab === 'appearance' }"
              @click="tab = 'appearance'"
            >
              外观
            </button>
            <button
              class="m3-tab m3-state-layer m3-ripple"
              :class="{ 'm3-tab--active': tab === 'dialog' }"
              @click="tab = 'dialog'"
            >
              对话
            </button>
          </div>

          <div class="m3-dialog__body !pt-4">
            <!-- ============ 外观 ============ -->
            <div v-if="tab === 'appearance'" class="flex flex-col gap-4">
              <!-- 后端地址 -->
              <div class="m3-field">
                <label class="m3-field__label">后端 API 地址</label>
                <div class="m3-field__box">
                  <input
                    v-model="url"
                    type="text"
                    class="m3-field__input"
                    placeholder="http://192.168.1.5:8081（留空 = 自动）"
                  />
                </div>
                <p class="m3-field__helper">前端与后端解耦，填写任意部署了 Alice 后端的地址即可。</p>
              </div>

              <div class="flex items-center justify-between">
                <span class="m3-label-medium m3-on-surface-variant">
                  当前连接: {{ chat.connected ? '已连接' : '未连接' }}
                </span>
                <span v-if="saved" class="m3-label-medium text-[var(--md-sys-color-tertiary)]">已保存并重连</span>
                <span v-if="connecting" class="m3-label-medium m3-on-surface-variant">重连中...</span>
              </div>

              <!-- 连接信息 -->
              <div v-if="chat.serverInfo" class="m3-card m3-card--filled">
                <div class="flex justify-between">
                  <span class="m3-label-medium m3-on-surface-variant">LLM</span>
                  <span class="m3-label-medium">{{ chat.serverInfo.llm }}</span>
                </div>
                <div class="mt-1 flex justify-between">
                  <span class="m3-label-medium m3-on-surface-variant">版本</span>
                  <span class="m3-label-medium">{{ chat.serverInfo.version }}</span>
                </div>
              </div>

              <!-- Material You 取色 -->
              <div>
                <div class="m3-section-title">外观（Material You 取色）</div>
                <div class="mt-2 flex items-center gap-2">
                  <label
                    class="m3-btn m3-btn--tonal m3-state-layer m3-ripple flex-1 cursor-pointer"
                  >
                    从壁纸取色
                    <input type="file" accept="image/*" class="hidden" @change="onWallpaperFile" />
                  </label>
                  <div
                    class="h-9 w-9 shrink-0 rounded-full border-2 border-[var(--md-sys-color-outline-variant)]"
                    :style="{ background: chat.seedColor }"
                    title="当前种子色"
                  />
                </div>
                <p v-if="wallpaperInfo" class="m3-label-small mt-1.5 text-[var(--md-sys-color-tertiary)]">
                  {{ wallpaperInfo }}
                </p>
              </div>

              <!-- 取色算法 -->
              <div>
                <div class="m3-section-title">取色算法</div>
                <div class="mt-1.5 flex flex-wrap gap-1.5">
                  <button
                    v-for="s in THEME_STYLES"
                    :key="s.id"
                    class="m3-chip"
                    :class="{ 'm3-chip--selected': chat.themeStyle === s.id }"
                    :title="s.desc"
                    @click="chat.setThemeStyle(s.id)"
                  >
                    {{ s.name }}
                  </button>
                </div>
              </div>

              <!-- 预设色板 -->
              <div>
                <div class="m3-section-title">预设色板</div>
                <div class="mt-1.5 flex flex-wrap gap-2">
                  <button
                    v-for="c in presets"
                    :key="c"
                    class="h-8 w-8 rounded-full border transition hover:scale-110"
                    :class="chat.seedColor === c ? 'ring-2 ring-[var(--md-sys-color-primary)]' : 'border-[var(--md-sys-color-outline-variant)]'"
                    :style="{ background: c }"
                    :title="c"
                    @click="chat.setSeedColor(c)"
                  />
                </div>
                <p class="m3-label-small m3-on-surface-variant mt-2">
                  动态取色会提取壁纸主色生成整套配色，刷新后保留。
                </p>
              </div>

              <div class="flex justify-end gap-2">
                <button class="m3-btn m3-btn--text m3-state-layer m3-ripple" @click="chat.settingsOpen = false">
                  取消
                </button>
                <button class="m3-btn m3-btn--outlined m3-state-layer m3-ripple" @click="useDefault">
                  恢复默认
                </button>
                <button
                  class="m3-btn m3-btn--filled m3-state-layer m3-ripple"
                  :disabled="connecting"
                  @click="save"
                >
                  {{ connecting ? '重连中...' : '保存并重连' }}
                </button>
              </div>
            </div>

            <!-- ============ 对话设置 ============ -->
            <div v-else class="flex flex-col gap-4">
              <p v-if="!settingsLoaded && !settingsErr" class="m3-label-medium m3-on-surface-variant">加载中...</p>
              <p v-if="settingsErr" class="m3-label-medium text-[var(--md-sys-color-error)]">{{ settingsErr }}</p>

              <template v-if="settingsLoaded">
                <!-- LLM -->
                <div class="m3-card m3-card--filled">
                  <div class="m3-title-small mb-3">LLM（对话模型）</div>
                  <div class="grid grid-cols-2 gap-3">
                    <div class="m3-field">
                      <label class="m3-field__label">Provider</label>
                      <div class="m3-field__box">
                        <select v-model="llmForm.provider" class="m3-field__input !p-0 bg-transparent">
                          <option v-for="p in providers" :key="p" :value="p" class="bg-[var(--md-sys-color-surface-container)]">{{ p }}</option>
                        </select>
                      </div>
                    </div>
                    <div class="m3-field">
                      <label class="m3-field__label">Model</label>
                      <div class="m3-field__box">
                        <input v-model="llmForm.model" type="text" class="m3-field__input" />
                      </div>
                    </div>
                  </div>
                  <div class="m3-field mt-3">
                    <label class="m3-field__label">Base URL（OpenAI 兼容）</label>
                    <div class="m3-field__box">
                      <input v-model="llmForm.base_url" type="text" class="m3-field__input" placeholder="https://api.deepseek.com/v1" />
                    </div>
                  </div>
                  <div class="m3-field mt-3">
                    <label class="m3-field__label">
                      API Key（留空 = 不修改{{ llmForm.api_key_configured ? '，当前已配置' : '，当前未配置（mock 模式）' }}）
                    </label>
                    <div class="m3-field__box">
                      <input
                        v-model="llmForm.api_key"
                        type="password"
                        class="m3-field__input"
                        :placeholder="llmForm.api_key_configured ? 'sk-***（已配置）' : 'sk-...'"
                      />
                    </div>
                  </div>
                  <div class="mt-3 grid grid-cols-2 gap-3">
                    <div class="m3-field">
                      <label class="m3-field__label">Temperature</label>
                      <div class="m3-field__box">
                        <input v-model.number="llmForm.temperature" type="number" step="0.1" min="0" max="2" class="m3-field__input" />
                      </div>
                    </div>
                    <div class="m3-field">
                      <label class="m3-field__label">Max Tokens</label>
                      <div class="m3-field__box">
                        <input v-model.number="llmForm.max_tokens" type="number" step="64" min="128" class="m3-field__input" />
                      </div>
                    </div>
                  </div>
                </div>

                <!-- 情绪 -->
                <div class="m3-card m3-card--filled">
                  <div class="m3-title-small mb-3">情绪引擎</div>
                  <div class="grid grid-cols-2 gap-3">
                    <div class="m3-field">
                      <label class="m3-field__label">主动推送阈值</label>
                      <div class="m3-field__box">
                        <input v-model.number="emotionForm.threshold" type="number" step="0.05" min="0" max="1" class="m3-field__input" />
                      </div>
                    </div>
                    <div class="m3-field">
                      <label class="m3-field__label">推送冷却（秒）</label>
                      <div class="m3-field__box">
                        <input v-model.number="emotionForm.cooldown_seconds" type="number" step="60" min="0" class="m3-field__input" />
                      </div>
                    </div>
                    <div class="m3-field">
                      <label class="m3-field__label">衰减率（每秒）</label>
                      <div class="m3-field__box">
                        <input v-model.number="emotionForm.decay_rate" type="number" step="0.0001" min="0" class="m3-field__input" />
                      </div>
                    </div>
                    <div class="m3-field">
                      <label class="m3-field__label">情绪上限</label>
                      <div class="m3-field__box">
                        <input v-model.number="emotionForm.max_value" type="number" step="0.1" min="0.1" max="2" class="m3-field__input" />
                      </div>
                    </div>
                    <div class="m3-field">
                      <label class="m3-field__label">静默触发（分钟，0=关）</label>
                      <div class="m3-field__box">
                        <input v-model.number="emotionForm.silent_after_minutes" type="number" step="5" min="0" class="m3-field__input" />
                      </div>
                    </div>
                    <div class="m3-field">
                      <label class="m3-field__label">活跃跳过（分钟）</label>
                      <div class="m3-field__box">
                        <input v-model.number="emotionForm.skip_if_active_minutes" type="number" step="5" min="0" class="m3-field__input" />
                      </div>
                    </div>
                    <div class="m3-field">
                      <label class="m3-field__label">允许推送时段起（时）</label>
                      <div class="m3-field__box">
                        <input v-model.number="emotionForm.hours_start" type="number" min="0" max="23" class="m3-field__input" />
                      </div>
                    </div>
                    <div class="m3-field">
                      <label class="m3-field__label">允许推送时段止（时）</label>
                      <div class="m3-field__box">
                        <input v-model.number="emotionForm.hours_end" type="number" min="0" max="23" class="m3-field__input" />
                      </div>
                    </div>
                  </div>
                </div>

                <!-- 记忆 -->
                <div class="m3-card m3-card--filled">
                  <div class="m3-title-small mb-3">记忆</div>
                  <div class="m3-field">
                    <label class="m3-field__label">Memory Block 上限（条，0 = 无限）</label>
                    <div class="m3-field__box">
                      <input v-model.number="blockForm.max_entries" type="number" step="10" min="0" class="m3-field__input" />
                    </div>
                  </div>
                </div>

                <!-- 语音合成（Edge TTS 图形化调整） -->
                <div class="m3-card m3-card--filled">
                  <div class="m3-title-small mb-1">语音合成（Edge TTS）</div>
                  <p class="m3-body-small m3-on-surface-variant mb-3">
                    调整 Alice 的声音：音色、语速、音高、音量。试听满意后保存生效。
                  </p>

                  <!-- 音色 -->
                  <label class="m3-field__label">音色</label>
                  <div class="m3-field__box mt-1">
                    <span class="m3-field__leading m3-icon">record_voice_over</span>
                    <select v-model="audioForm.voice" class="m3-field__input !p-0 bg-transparent">
                      <option value="">默认（自动选择）</option>
                      <optgroup v-for="[group, list] in voiceGroups" :key="group" :label="group">
                        <option v-for="v in list" :key="v.short_name" :value="v.short_name">
                          {{ v.friendly_name }} · {{ v.short_name }}
                        </option>
                      </optgroup>
                    </select>
                  </div>
                  <p v-if="voiceLoading" class="m3-label-small m3-on-surface-variant mt-1">
                    音色列表加载中...
                  </p>

                  <!-- 语速 -->
                  <div class="mt-3">
                    <div class="flex items-center justify-between">
                      <span class="m3-label-medium">语速</span>
                      <span class="m3-label-medium m3-primary-text tabular-nums">{{ fmtSigned(audioForm.rate, '%') }}</span>
                    </div>
                    <input
                      v-model.number="audioForm.rate"
                      type="range"
                      min="-50"
                      max="100"
                      step="5"
                      class="m3-slider mt-1"
                    />
                  </div>

                  <!-- 音高 -->
                  <div class="mt-3">
                    <div class="flex items-center justify-between">
                      <span class="m3-label-medium">音高</span>
                      <span class="m3-label-medium m3-primary-text tabular-nums">{{ fmtSigned(audioForm.pitch, 'Hz') }}</span>
                    </div>
                    <input
                      v-model.number="audioForm.pitch"
                      type="range"
                      min="-20"
                      max="20"
                      step="1"
                      class="m3-slider mt-1"
                    />
                  </div>

                  <!-- 音量 -->
                  <div class="mt-3">
                    <div class="flex items-center justify-between">
                      <span class="m3-label-medium">音量</span>
                      <span class="m3-label-medium m3-primary-text tabular-nums">{{ fmtSigned(audioForm.volume, '%') }}</span>
                    </div>
                    <input
                      v-model.number="audioForm.volume"
                      type="range"
                      min="-50"
                      max="50"
                      step="5"
                      class="m3-slider mt-1"
                    />
                  </div>

                  <audio ref="previewAudio" class="hidden" />
                  <p v-if="previewMsg" class="m3-label-small mt-2 text-[var(--md-sys-color-tertiary)]">{{ previewMsg }}</p>
                  <p v-if="previewErr || audioErr" class="m3-label-small mt-2 text-[var(--md-sys-color-error)]">{{ previewErr || audioErr }}</p>
                  <p v-if="audioMsg" class="m3-label-small mt-2 text-[var(--md-sys-color-tertiary)]">{{ audioMsg }}</p>

                  <div class="mt-3 flex justify-end gap-2">
                    <button
                      class="m3-btn m3-btn--tonal m3-state-layer m3-ripple"
                      :disabled="previewing"
                      @click="previewTTS"
                    >
                      <span v-if="previewing" class="m3-spinner h-4 w-4 border-2" />
                      <span v-else class="m3-icon m3-icon--sm">volume_up</span>
                      试听
                    </button>
                    <button
                      class="m3-btn m3-btn--filled m3-state-layer m3-ripple"
                      :disabled="savingAudio"
                      @click="saveAudio"
                    >
                      <span v-if="savingAudio" class="m3-spinner h-4 w-4 border-2" />
                      保存语音设置
                    </button>
                  </div>
                </div>

                <p v-if="settingsMsg" class="m3-label-medium text-[var(--md-sys-color-tertiary)]">{{ settingsMsg }}</p>
                <p v-if="settingsErr" class="m3-label-medium text-[var(--md-sys-color-error)]">{{ settingsErr }}</p>

                <div class="flex justify-end">
                  <button
                    class="m3-btn m3-btn--filled m3-state-layer m3-ripple"
                    :disabled="savingSettings"
                    @click="applySettings"
                  >
                    <span v-if="savingSettings" class="m3-spinner h-4 w-4 border-2" />
                    {{ savingSettings ? '保存中...' : '保存设置' }}
                  </button>
                </div>
              </template>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
