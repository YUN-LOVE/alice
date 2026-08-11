<script setup lang="ts">
import { ref } from 'vue'
import { useChatStore } from '../../stores/chat'
import { THEME_STYLES } from '../../styles/theme'
import { ws } from '../../services/ws'

const chat = useChatStore()
const url = ref(chat.backendUrl)
const saved = ref(false)
const connecting = ref(false)
const wallpaperInfo = ref('')

// Material You 预设种子色
const presets = [
  '#7c5cff', // 动态紫（默认）
  '#6750a4', // 经典紫
  '#00639a', // 蓝
  '#006c5c', // 绿
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
  // 等待握手确认（或连接失败）后关闭面板
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
    setTimeout(() => chat.settingsOpen = false, 600)
  }
}

function useDefault() {
  url.value = ''
  save()
}
</script>

<template>
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
    @click.self="chat.settingsOpen = false"
  >
    <div
      class="max-h-[92dvh] w-full max-w-[420px] overflow-y-auto rounded-2xl border border-zinc-500 bg-zinc-900 p-5 shadow-2xl"
    >
      <h2 class="text-base font-medium text-zinc-100">设置</h2>

      <div class="mt-4">
        <label class="text-xs text-zinc-400">后端 API 地址</label>
        <input
          v-model="url"
          type="text"
          placeholder="http://192.168.1.5:8081（留空 = 自动）"
          class="mt-1 w-full rounded-lg border border-zinc-500 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 placeholder-zinc-600 focus:border-purple-500 focus:outline-none"
        />
        <p class="mt-1 text-xs text-zinc-500">
          前端与后端完全解耦，填写任意部署了 Alice 后端的地址即可，HTTP 与 WebSocket 由此自动推导。
        </p>
      </div>

      <div class="mt-2 flex items-center justify-between text-xs text-zinc-500">
        <span>当前连接: {{ chat.connected ? '已连接' : '未连接' }}</span>
        <span v-if="saved" class="text-emerald-400">已保存并重连</span>
        <span v-if="connecting" class="text-zinc-400">重连中...</span>
      </div>

      <!-- 连接信息 -->
      <div v-if="chat.serverInfo" class="mt-3 rounded-lg border border-zinc-500 bg-zinc-950 p-3 text-xs">
        <div class="flex justify-between">
          <span class="text-zinc-500">LLM</span>
          <span class="text-zinc-300">{{ chat.serverInfo.llm }}</span>
        </div>
        <div class="mt-1 flex justify-between">
          <span class="text-zinc-500">版本</span>
          <span class="text-zinc-300">{{ chat.serverInfo.version }}</span>
        </div>
      </div>

      <!-- 外观：Material You 取色 -->
      <div class="mt-4">
        <label class="text-xs m3-on-surface-variant">外观（Material You 取色）</label>

        <!-- 壁纸取色 -->
        <div class="mt-1.5 flex items-center gap-2">
          <label
            class="m3-primary-container flex-1 cursor-pointer rounded-full px-4 py-2 text-center text-xs"
          >
            从壁纸取色
            <input type="file" accept="image/*" class="hidden" @change="onWallpaperFile" />
          </label>
          <div
            class="h-8 w-8 rounded-full border border-zinc-500"
            :style="{ background: chat.seedColor }"
            title="当前种子色"
          />
        </div>
        <p v-if="wallpaperInfo" class="mt-1 text-[10px] text-emerald-400">{{ wallpaperInfo }}</p>

        <!-- 取色算法 -->
        <div class="mt-2.5">
          <div class="mb-1 text-[10px] m3-on-surface-variant">取色算法</div>
          <div class="flex flex-wrap gap-1">
            <button
              v-for="s in THEME_STYLES"
              :key="s.id"
              class="rounded-full px-2.5 py-1 text-[11px] transition"
              :class="
                chat.themeStyle === s.id
                  ? 'm3-secondary-container'
                  : 'm3-surface-container-highest m3-on-surface-variant hover:opacity-80'
              "
              :title="s.desc"
              @click="chat.setThemeStyle(s.id)"
            >
              {{ s.name }}
            </button>
          </div>
        </div>

        <!-- 预设色板 -->
        <div class="mt-2 flex flex-wrap gap-1.5">
          <button
            v-for="c in presets"
            :key="c"
            class="h-6 w-6 rounded-full border transition hover:scale-110"
            :class="chat.seedColor === c ? 'ring-2 ring-zinc-300' : 'border-zinc-600'"
            :style="{ background: c }"
            @click="chat.setSeedColor(c)"
          />
        </div>
        <p class="mt-1 text-[10px] m3-on-surface-variant">
          动态取色会提取壁纸主色生成整套配色，刷新后保留。
        </p>
      </div>

      <div class="mt-5 flex justify-end gap-2">
        <button
          class="rounded-lg px-3 py-1.5 text-sm text-zinc-400 hover:bg-zinc-800"
          @click="chat.settingsOpen = false"
        >
          取消
        </button>
        <button
          class="rounded-lg bg-zinc-700 px-3 py-1.5 text-sm text-zinc-200 hover:bg-zinc-600"
          @click="useDefault"
        >
          恢复默认
        </button>
        <button
          class="rounded-lg bg-purple-600 px-4 py-1.5 text-sm text-white hover:bg-purple-500"
          :disabled="connecting"
          @click="save"
        >
          保存并重连
        </button>
      </div>
    </div>
  </div>
</template>
