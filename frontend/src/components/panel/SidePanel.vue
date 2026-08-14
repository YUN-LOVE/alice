<script setup lang="ts">
import { watch } from 'vue'
import { useChatStore } from '../../stores/chat'
import { useMemoryStore } from '../../stores/memory'
import { useMCPStore } from '../../stores/mcp'
import MemoryPanel from '../memory/MemoryPanel.vue'
import MCPPanel from '../mcp/MCPPanel.vue'
import EmotionPanel from '../emotion/EmotionPanel.vue'

const chat = useChatStore()
const memory = useMemoryStore()
const mcp = useMCPStore()

const tabs = [
  { id: 'memory', label: '记忆', icon: 'psychology' },
  { id: 'mcp', label: 'MCP', icon: 'extension' },
  { id: 'emotion', label: '情绪', icon: 'sentiment_satisfied' },
]

// 切换 tab 时懒加载对应数据
watch(
  () => chat.panelOpen && chat.panelTab,
  (v) => {
    if (v === 'memory') void memory.refresh()
    if (v === 'mcp') void mcp.refresh()
  }
)
</script>

<template>
  <Teleport to="body">
    <Transition name="m3-fade">
      <div
        v-if="chat.panelOpen"
        class="m3-scrim"
        @click="chat.panelOpen = false"
      />
    </Transition>

    <Transition name="m3-drawer">
      <aside
        v-if="chat.panelOpen"
        class="m3-drawer"
        role="dialog"
        aria-label="侧边面板"
      >
        <!-- 精简头部：Tabs 导航 + 关闭（不再是"第二个顶栏"） -->
        <div class="m3-drawer__head">
          <div class="m3-tabs flex-1 border-b-0">
            <button
              v-for="t in tabs"
              :key="t.id"
              class="m3-tab m3-state-layer m3-ripple"
              :class="{ 'm3-tab--active': chat.panelTab === t.id }"
              @click="chat.panelTab = t.id"
            >
              <span class="m3-icon m3-icon--sm">{{ t.icon }}</span>
              {{ t.label }}
            </button>
          </div>
          <button
            class="m3-icon-btn m3-state-layer m3-ripple shrink-0"
            title="关闭"
            @click="chat.panelOpen = false"
          >
            <span class="m3-icon">close</span>
          </button>
        </div>

        <!-- 内容 -->
        <div class="m3-drawer__body">
          <MemoryPanel v-if="chat.panelTab === 'memory'" />
          <MCPPanel v-if="chat.panelTab === 'mcp'" />
          <EmotionPanel v-if="chat.panelTab === 'emotion'" />
        </div>
      </aside>
    </Transition>
  </Teleport>
</template>
