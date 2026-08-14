<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useMCPStore } from '../../stores/mcp'

const mcp = useMCPStore()
const tab = ref<'installed' | 'market'>('installed')

onMounted(() => {
  void mcp.refresh()
  void mcp.refreshMarket()
})

// ============ 配置编辑（M3 Dialog） ============
const editId = ref('')
const editForm = ref({ args: '', env: '', url: '', enabled: true })

function openEdit(id: string) {
  const s = mcp.servers.find((x) => x.id === id)
  if (!s) return
  editId.value = id
  editForm.value = { args: '', env: '', url: '', enabled: s.running }
}

function saveEdit() {
  const args = editForm.value.args
    .split(/\s+/)
    .map((s) => s.trim())
    .filter(Boolean)
  const env = editForm.value.env
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)
  mcp.configure(editId.value, {
    args,
    env,
    url: editForm.value.url.trim(),
    enabled: editForm.value.enabled,
  })
  editId.value = ''
}
</script>

<template>
  <div class="flex flex-col gap-3">
    <!-- 切换：已安装 / 市场（M3 Tabs） -->
    <div class="m3-tabs">
      <button
        class="m3-tab m3-state-layer m3-ripple"
        :class="{ 'm3-tab--active': tab === 'installed' }"
        @click="tab = 'installed'"
      >
        已安装 ({{ mcp.servers.length }})
      </button>
      <button
        class="m3-tab m3-state-layer m3-ripple"
        :class="{ 'm3-tab--active': tab === 'market' }"
        @click="tab = 'market'; mcp.refreshMarket()"
      >
        市场 ({{ mcp.market.length }})
      </button>
    </div>

    <div v-if="mcp.error" class="m3-label-medium text-[var(--md-sys-color-error)]">{{ mcp.error }}</div>

    <!-- 已安装 -->
    <div v-if="tab === 'installed'" class="flex flex-col gap-3">
      <div v-if="mcp.loading" class="flex items-center gap-2 py-2">
        <span class="m3-spinner h-4 w-4" />
        <span class="m3-label-medium m3-on-surface-variant">加载中...</span>
      </div>
      <div v-if="!mcp.loading && mcp.servers.length === 0" class="m3-body-medium m3-on-surface-variant py-6 text-center">
        暂无 MCP，去「市场」安装
      </div>

      <div v-for="s in mcp.servers" :key="s.id" class="m3-card m3-card--elevated">
        <!-- Server 行 -->
        <div class="flex items-center justify-between gap-2">
          <div class="min-w-0">
            <div class="m3-title-small flex items-center gap-2">
              <span class="truncate">{{ s.name }}</span>
              <span
                class="inline-block h-2 w-2 shrink-0 rounded-full"
                :class="s.running ? 'bg-[var(--md-sys-color-tertiary)]' : 'bg-[var(--md-sys-color-outline)]'"
                :title="s.running ? '运行中' : '已停止'"
              />
            </div>
            <div class="m3-label-small m3-on-surface-variant mt-0.5">{{ s.id }}</div>
          </div>
          <div class="flex shrink-0 items-center gap-1">
            <button
              class="m3-icon-btn m3-icon-btn--xs m3-state-layer m3-ripple"
              title="修改配置（args/env/url）"
              @click="openEdit(s.id)"
            >
              <span class="m3-icon m3-icon--sm">settings</span>
            </button>
            <label class="m3-switch m3-switch--sm shrink-0">
              <input
                type="checkbox"
                :checked="s.running"
                @change="mcp.toggle(s.id, !s.running)"
              />
              <span class="m3-switch__track" />
              <span class="m3-switch__thumb" />
            </label>
          </div>
        </div>

        <!-- 工具级开关 -->
        <div v-if="(s.tools?.length ?? 0) > 0" class="mt-3 border-t border-[var(--md-sys-color-outline-variant)] pt-2">
          <div class="m3-section-title !px-1">工具 ({{ s.tools.length }})</div>
          <div v-for="t in s.tools ?? []" :key="t.name" class="m3-list-item !min-h-[40px] !px-1 m3-state-layer">
            <div class="m3-list-item__text">
              <span class="m3-list-item__subtitle !text-[13px] !text-[var(--md-sys-color-on-surface)] font-mono">{{ t.name }}</span>
            </div>
            <label class="m3-switch m3-switch--sm shrink-0">
              <input
                type="checkbox"
                :checked="t.enabled"
                @change="mcp.toggleTool(s.id, t.name, !t.enabled)"
              />
              <span class="m3-switch__track" />
              <span class="m3-switch__thumb" />
            </label>
          </div>
        </div>
      </div>
    </div>

    <!-- 市场 -->
    <div v-else class="flex flex-col gap-2">
      <div v-if="mcp.market.length === 0" class="m3-body-medium m3-on-surface-variant py-6 text-center">
        市场暂无条目
      </div>
      <div v-for="it in mcp.market" :key="it.id" class="m3-card m3-card--outlined">
        <div class="flex items-center justify-between gap-2">
          <div class="min-w-0">
            <div class="m3-title-small truncate">{{ it.name }}</div>
            <div class="m3-label-small m3-on-surface-variant mt-0.5">{{ it.id }}</div>
          </div>
          <button
            class="m3-btn m3-btn--sm shrink-0"
            :class="it.installed ? 'm3-btn--outlined' : 'm3-btn--filled m3-state-layer m3-ripple'"
            @click="it.installed ? mcp.uninstall(it.id) : mcp.install(it.id)"
          >
            {{ it.installed ? '卸载' : '安装' }}
          </button>
        </div>
        <p class="m3-body-small m3-on-surface-variant mt-1.5 leading-relaxed">{{ it.description }}</p>
      </div>
    </div>

    <!-- 配置 Dialog -->
    <Teleport to="body">
      <Transition name="m3-fade">
        <div v-if="editId" class="m3-dialog-overlay" @click.self="editId = ''">
          <div class="m3-dialog m3-dialog--full m3-dialog-enter" role="dialog" aria-label="修改 MCP 配置">
            <div class="m3-dialog__head">
              <div class="m3-dialog__title">修改配置</div>
            </div>
            <div class="m3-dialog__body">
              <p class="m3-label-medium m3-on-surface-variant mb-3">Server: {{ editId }}</p>

              <div class="m3-field">
                <label class="m3-field__label">启动参数 args（空格分隔）</label>
                <div class="m3-field__box">
                  <input v-model="editForm.args" type="text" class="m3-field__input" placeholder="如：--port 8080" />
                </div>
              </div>

              <div class="m3-field mt-3">
                <label class="m3-field__label">环境变量 env（每行 KEY=VALUE）</label>
                <div class="m3-field__box">
                  <textarea v-model="editForm.env" rows="3" class="m3-field__input resize-none" placeholder="TAVILY_API_KEY=tvly-xxx" />
                </div>
              </div>

              <div class="m3-field mt-3">
                <label class="m3-field__label">URL（http 传输时）</label>
                <div class="m3-field__box">
                  <input v-model="editForm.url" type="text" class="m3-field__input" placeholder="https://mcp.example.com" />
                </div>
              </div>

              <label class="mt-4 flex items-center justify-between rounded-[12px] bg-[var(--md-sys-color-surface-container-highest)] px-4 py-3">
                <span class="m3-label-medium">启用</span>
                <span class="m3-switch">
                  <input v-model="editForm.enabled" type="checkbox" />
                  <span class="m3-switch__track" />
                  <span class="m3-switch__thumb" />
                </span>
              </label>
            </div>
            <div class="m3-dialog__actions">
              <button class="m3-btn m3-btn--text m3-state-layer m3-ripple" @click="editId = ''">取消</button>
              <button class="m3-btn m3-btn--filled m3-state-layer m3-ripple" @click="saveEdit">保存（热重载）</button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
