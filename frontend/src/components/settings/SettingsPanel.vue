<script setup lang="ts">
import { ref } from 'vue'
import { useChatStore } from '../../stores/chat'
import { ws } from '../../services/ws'

const chat = useChatStore()
const url = ref(chat.backendUrl)
const saved = ref(false)
const connecting = ref(false)

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
    <div class="w-[420px] rounded-2xl border border-zinc-800 bg-zinc-900 p-5 shadow-2xl">
      <h2 class="text-base font-medium text-zinc-100">设置</h2>

      <div class="mt-4">
        <label class="text-xs text-zinc-400">后端 API 地址</label>
        <input
          v-model="url"
          type="text"
          placeholder="http://192.168.1.5:8081（留空 = 自动）"
          class="mt-1 w-full rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 placeholder-zinc-600 focus:border-purple-500 focus:outline-none"
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
