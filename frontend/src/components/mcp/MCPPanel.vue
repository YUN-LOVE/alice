<script setup lang="ts">
import { useMCPStore } from '../../stores/mcp'

const mcp = useMCPStore()
</script>

<template>
  <div class="flex flex-col gap-2">
    <div class="flex items-center justify-between">
      <span class="text-xs text-zinc-500">已安装 MCP Server</span>
      <button class="text-xs text-zinc-500 hover:text-zinc-300" @click="mcp.refresh()">刷新</button>
    </div>

    <div v-if="mcp.error" class="text-xs text-red-400">{{ mcp.error }}</div>
    <div v-if="mcp.loading" class="text-xs text-zinc-500">加载中...</div>
    <div v-if="!mcp.loading && mcp.servers.length === 0" class="text-xs text-zinc-600">
      暂无 MCP，在 config/mcp.yaml 中注册后重启
    </div>

    <div
      v-for="s in mcp.servers"
      :key="s.id"
      class="rounded-lg border border-zinc-800 bg-zinc-950 p-3"
    >
      <div class="flex items-center justify-between">
        <div>
          <div class="text-sm text-zinc-100">{{ s.name }}</div>
          <div class="text-xs text-zinc-500">{{ s.id }}</div>
        </div>
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
      <div class="mt-2 flex items-center gap-2 text-xs text-zinc-500">
        <span
          class="inline-block h-1.5 w-1.5 rounded-full"
          :class="s.running ? 'bg-emerald-400' : 'bg-zinc-600'"
        />
        {{ s.running ? '运行中' : '已停止' }} · {{ s.tool_count }} 个工具
      </div>
    </div>
  </div>
</template>
