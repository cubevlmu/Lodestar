<script setup lang="ts">
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import { useToast } from 'primevue/usetoast'
import { onMounted, reactive, ref } from 'vue'
import CopyButton from '../components/CopyButton.vue'
import { api } from '../services/api'
import type { AuditLog, ServerSettings } from '../types'
import { formatBytes, formatDateTime, formatUptime } from '../utils'

const toast = useToast()

const settings = ref<ServerSettings | null>(null)
const audit = ref<AuditLog[]>([])
const loading = ref(true)
const error = ref('')

const passwordForm = reactive({ current: '', next: '', confirm: '' })
const passwordBusy = ref(false)
const passwordError = ref('')

const baseUrl = ref('')
const baseUrlBusy = ref(false)

const agentSnippet = `# 在每台生产服务器上：
#  1) 复制 lodestar 可执行文件到 /opt/lodestar
#  2) 在「目标主机」中登记并复制生成的配置
#  3) 用 systemd 托管
[Unit]
Description=Lodestar Agent
After=network-online.target

[Service]
WorkingDirectory=/opt/lodestar
ExecStart=/opt/lodestar/lodestar agent
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target`

async function load() {
  loading.value = true
  error.value = ''
  try {
    settings.value = await api.settings()
    baseUrl.value = settings.value.public_base_url
    const logs = await api.auditLogs()
    audit.value = logs.items
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function changePassword() {
  passwordError.value = ''
  if (passwordForm.next.length < 8) {
    passwordError.value = '新密码至少 8 位。'
    return
  }
  if (passwordForm.next !== passwordForm.confirm) {
    passwordError.value = '两次输入的新密码不一致。'
    return
  }
  passwordBusy.value = true
  try {
    await api.changePassword(passwordForm.current, passwordForm.next)
    toast.add({ severity: 'success', summary: '密码已更新', detail: '其他设备上的会话已失效。', life: 3200 })
    Object.assign(passwordForm, { current: '', next: '', confirm: '' })
  } catch (reason) {
    passwordError.value = reason instanceof Error ? reason.message : '修改失败'
  } finally {
    passwordBusy.value = false
  }
}

async function saveBaseUrl() {
  baseUrlBusy.value = true
  try {
    await api.saveSettings(baseUrl.value.trim())
    toast.add({ severity: 'success', summary: '已保存', life: 2200 })
    await load()
  } catch (reason) {
    toast.add({ severity: 'error', summary: '保存失败', detail: reason instanceof Error ? reason.message : '', life: 4000 })
  } finally {
    baseUrlBusy.value = false
  }
}

onMounted(load)
</script>

<template>
  <Message v-if="error" severity="error" :closable="false" class="mb-4">{{ error }}</Message>

  <div class="grid gap-4 xl:grid-cols-[1.1fr_1fr]">
    <div class="space-y-4">
      <section class="surface-panel rounded-lg p-4 sm:p-5">
        <h2 class="text-sm font-semibold text-color">服务信息</h2>
        <p class="mt-1 text-xs text-muted-color">当前 Lodestar 实例的运行参数。</p>
        <div v-if="settings" class="mt-3">
          <div class="detail-row"><span>版本</span><strong class="mono">v{{ settings.version }} · {{ settings.commit }}</strong></div>
          <div class="detail-row"><span>运行时长</span><strong>{{ formatUptime(settings.uptime) }}</strong></div>
          <div class="detail-row"><span>启动时间</span><strong>{{ formatDateTime(settings.started_at) }}</strong></div>
          <div class="detail-row"><span>版本 / 任务</span><strong>{{ settings.release_count }} / {{ settings.job_count }}</strong></div>
          <div class="detail-row"><span>单包上限</span><strong>{{ formatBytes(settings.max_upload_bytes) }}</strong></div>
          <div class="detail-row"><span>数据目录</span><strong class="mono">{{ settings.data_dir }}</strong></div>
          <div class="detail-row"><span>数据库</span><strong class="mono">{{ settings.db_path }}</strong></div>
          <div class="detail-row"><span>存储目录</span><strong class="mono">{{ settings.storage_dir }}</strong></div>
        </div>
      </section>

      <section class="surface-panel rounded-lg p-4 sm:p-5">
        <h2 class="text-sm font-semibold text-color">对外地址</h2>
        <p class="mt-1 text-xs text-muted-color">客户端下载链接与 CI 调用使用的公网地址。留空则自动使用请求 Host。</p>
        <div class="mt-3 flex flex-col gap-2 sm:flex-row">
          <InputText v-model="baseUrl" placeholder="https://lodestar.example.com" fluid />
          <Button :loading="baseUrlBusy" class="shrink-0" @click="saveBaseUrl">保存</Button>
        </div>
        <p v-if="settings" class="mt-2 text-xs text-muted-color">
          当前生效：<span class="mono">{{ settings.effective_base_url }}</span>
        </p>
      </section>

      <section class="surface-panel rounded-lg p-4 sm:p-5">
        <h2 class="text-sm font-semibold text-color">修改密码</h2>
        <div class="mt-3 flex flex-col gap-3">
          <div>
            <label class="field-label">当前密码</label>
            <Password v-model="passwordForm.current" :feedback="false" toggle-mask autocomplete="current-password" fluid input-class="w-full" />
          </div>
          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label class="field-label">新密码</label>
              <Password v-model="passwordForm.next" :feedback="false" toggle-mask autocomplete="new-password" fluid input-class="w-full" />
            </div>
            <div>
              <label class="field-label">确认新密码</label>
              <Password v-model="passwordForm.confirm" :feedback="false" toggle-mask autocomplete="new-password" fluid input-class="w-full" />
            </div>
          </div>
          <Message v-if="passwordError" severity="error" size="small">{{ passwordError }}</Message>
          <div><Button :loading="passwordBusy" size="small" @click="changePassword">更新密码</Button></div>
        </div>
      </section>
    </div>

    <div class="space-y-4">
      <section class="surface-panel rounded-lg p-4 sm:p-5">
        <div class="flex items-start justify-between gap-3">
          <div>
            <h2 class="text-sm font-semibold text-color">Agent 部署</h2>
            <p class="mt-1 text-xs text-muted-color">同一个可执行文件既能做服务端，也能做生产主机上的 Agent。</p>
          </div>
          <CopyButton :value="agentSnippet" label="复制" />
        </div>
        <div class="code-block mt-3">{{ agentSnippet }}</div>
      </section>

      <section class="surface-panel rounded-lg">
        <div class="flex items-center justify-between border-b border-surface-200 px-4 py-3 dark:border-surface-800">
          <h2 class="text-sm font-semibold text-color">最近操作</h2>
          <span class="text-xs text-muted-color">审计日志</span>
        </div>
        <DataTable :value="audit" :loading="loading" data-key="id" size="small" class="text-sm">
          <template #empty><div class="table-empty-state">还没有操作记录。</div></template>
          <Column header="时间" class="hidden sm:table-cell" style="width: 9rem">
            <template #body="{ data: item }"><span class="text-xs text-muted-color">{{ formatDateTime(item.created_at) }}</span></template>
          </Column>
          <Column header="操作">
            <template #body="{ data: item }"><span class="mono text-xs text-color">{{ item.action }}</span></template>
          </Column>
          <Column header="对象" class="hidden md:table-cell">
            <template #body="{ data: item }"><span class="line-clamp-1 text-xs text-muted-color">{{ item.detail || '—' }}</span></template>
          </Column>
        </DataTable>
      </section>

      <section class="surface-panel rounded-lg p-4 sm:p-5">
        <h2 class="text-sm font-semibold text-color">关于</h2>
        <p class="mt-2 text-xs leading-5 text-muted-color">
          Lodestar 是一个自托管的发布与远程部署控制台：登记版本包、下发部署任务、由生产主机上的 Agent 完成下载校验、备份替换与重启，
          失败时自动回滚。前端基于 PrimeVue 5（PrimeUI 社区许可），界面布局沿用 SiriusNet 控制台。
        </p>
        <p class="mt-2 text-xs text-muted-color">代码以 MIT 许可发布，详见仓库根目录 <span class="mono">LICENSE</span>。</p>
      </section>
    </div>
  </div>
</template>

<style scoped>
.detail-row {
  display: grid;
  grid-template-columns: minmax(6rem, 0.55fr) minmax(0, 1.45fr);
  gap: 1rem;
  border-bottom: 1px solid var(--p-content-border-color);
  padding: 0.5rem 0;
  font-size: 0.8125rem;
}

.detail-row:last-child {
  border-bottom: 0;
}

.detail-row > span {
  color: var(--p-text-muted-color);
}

.detail-row > strong {
  min-width: 0;
  overflow-wrap: anywhere;
  text-align: right;
  font-weight: 600;
  color: var(--p-text-color);
}
</style>
