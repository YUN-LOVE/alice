<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useMCPStore } from '../../stores/mcp'

const mcp = useMCPStore()
const tab = ref<'installed' | 'market'>('installed')

onMounted(() => {
  void mcp.refresh()
  void mcp.refreshMarket()
})
</script>

<template>
  <div class="flex flex-col gap-3">
    <!-- 切换：已安装 / 市场 -->
    <div class="flex gap-1">
      <button
        class="rounded-lg px-3 py-1.5 text-xs"
        :class="tab === 'installed' ? 'bg-purple-600 text-white' : 'text-zinc-400 hover:bg-zinc-800'"
        @click="tab = 'installed'"
      >
        已安装 ({{ mcp.servers.length }})
      </button>
      <button
        class="rounded-lg px-3 py-1.5 text-xs"
        :class="tab === 'market' ? 'bg-purple-600 text-white' : 'text-zinc-400 hover:bg-zinc-800'"
        @click="tab = 'market'; mcp.refreshMarket()"
      >
        市场 ({{ mcp.market.length }})
      </button>
    </div>

    <div v-if="mcp.error" class="text-xs text-red-400">{{ mcp.error }}</div>

    <!-- 已安装 -->
    <div v-if="tab === 'installed'" class="flex flex-col gap-3">
      <div v-if="mcp.loading" class="text-xs text-zinc-500">加载中...</div>
      <div v-if="!mcp.loading && mcp.servers.length === 0" class="text-xs text-zinc-600">
        暂无 MCP，去「市场」安装
      </div>

      <div v-for="s in mcp.servers" :key="s.id" class="rounded-lg border border-zinc-500 bg-zinc-950 p-3">
        <!-- Server 行 -->
        <div class="flex items-center justify-between">
          <div>
            <div class="text-sm text-zinc-100">{{ s.name }}</div>
            <div class="text-xs text-zinc-500">{{ s.id }}</div>
          </div>
          <div class="flex items-center gap-2">
            <span
              class="inline-block h-1.5 w-1.5 rounded-full"
              :class="s.running ? 'bg-emerald-400' : 'bg-zinc-600'"
            />
            <button
              class="relative h-5 w-9 rounded-full transition"
              :class="s.running ? 'bg-emerald-500' : 'bg-zinc-700'"
              role="switch"
              :aria-checked="s.running"
              @click="mcp.toggle(s.id, !s.running)"
            >
              <span
                class="absolute top-0.5 h-4 w-4 rounded-full bg-white transition"
                :class="s.running ? 'left-[18px]' : 'left-0.5'"
              />
            </button>
          </div>
        </div>

        <!-- 工具级开关 -->
        <div class="mt-2 border-t border-zinc-500 pt-2">
          <div class="mb-1 text-xs text-zinc-500">工具（{{ s.tools?.length ?? 0 }}）</div>
          <div v-for="t in s.tools ?? []" :key="t.name" class="flex items-center justify-between py-0.5">
            <span class="text-xs text-zinc-300">{{ t.name }}</span>
            <button
              class="relative h-4 w-7 rounded-full transition"
              :class="t.enabled ? 'bg-purple-500' : 'bg-zinc-700'"
              role="switch"
              :aria-checked="t.enabled"
              @click="mcp.toggleTool(s.id, t.name, !t.enabled)"
            >
              <span
                class="absolute top-0.5 h-3 w-3 rounded-full bg-white transition"
                :class="t.enabled ? 'left-[14px]' : 'left-0.5'"
              />
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 市场 -->
    <div v-else class="flex flex-col gap-2">
      <div v-if="mcp.market.length === 0" class="text-xs text-zinc-600">市场暂无条目</div>
      <div v-for="it in mcp.market" :key="it.id" class="rounded-lg border border-zinc-500 bg-zinc-950 p-3">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-sm text-zinc-100">{{ it.name }}</div>
            <div class="text-xs text-zinc-500">{{ it.id }}</div>
          </div>
          <button
            class="rounded-lg px-3 py-1 text-xs"
            :class="it.installed
              ? 'bg-zinc-800 text-zinc-400 hover:bg-zinc-700'
              : 'bg-purple-600 text-white hover:bg-purple-500'"
            @click="it.installed ? mcp.uninstall(it.id) : mcp.install(it.id)"
          >
            {{ it.installed ? '卸载' : '安装' }}
          </button>
        </div>
        <p class="mt-1 text-xs text-zinc-500">{{ it.description }}</p>
      </div>
    </div>
  </div>
</template>
