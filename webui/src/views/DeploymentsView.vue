<script setup lang="ts">
import Replay from '@primeicons/vue/replay'
import Bolt from '@primeicons/vue/bolt'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Drawer from 'primevue/drawer'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Button from 'primevue/button'
import Select from 'primevue/select'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import LogViewer from '../components/LogViewer.vue'
import StatusTag from '../components/StatusTag.vue'
import { api } from '../services/api'
import type { DeployJob, DeployLog, Project } from '../types'
import { formatDateTime, formatDuration, platformLabel, triggerLabel } from '../utils'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()

const jobs = ref<DeployJob[]>([])
const projects = ref<Project[]>([])
const total = ref(0)
const loading = ref(true)
const error = ref('')
const page = ref(1)
const pageSize = 20
const filters = reactive<{ project_id: number | null; status: string; q: string }>({ project_id: null, status: 'all', q: '' })

const projectOptions = computed(() => projects.value.map(project => ({ label: `${project.name} · ${project.app_id}`, value: project.id })))
const statusOptions = [
  { label: '全部状态', value: 'all' },
  { label: '排队中', value: 'pending' },
  { label: '执行中', value: 'running' },
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' },
  { label: '已取消', value: 'canceled' },
]

const detailVisible = ref(false)
const detailJob = ref<DeployJob | null>(null)
const detailLogs = ref<DeployLog[]>([])
const detailLoading = ref(false)
const actionBusy = ref(false)
let pollTimer = 0

const detailProject = computed(() => projects.value.find(project => project.id === detailJob.value?.project_id))
const isActive = computed(() => detailJob.value?.status === 'pending' || detailJob.value?.status === 'running')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const response = await api.jobs({
      project_id: filters.project_id ?? undefined,
      status: filters.status,
      q: filters.q || undefined,
      page: page.value,
      size: pageSize,
    })
    jobs.value = response.items
    total.value = response.total
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function openJob(id: number) {
  detailVisible.value = true
  detailLoading.value = true
  detailLogs.value = []
  stopPolling()
  try {
    const response = await api.job(id)
    detailJob.value = response.job
    detailLogs.value = response.logs
    if (response.job.status === 'pending' || response.job.status === 'running') startPolling()
  } catch (reason) {
    toast.add({ severity: 'error', summary: '加载任务失败', detail: reason instanceof Error ? reason.message : '', life: 4000 })
  } finally {
    detailLoading.value = false
  }
}

function startPolling() {
  stopPolling()
  pollTimer = window.setInterval(async () => {
    const job = detailJob.value
    if (!job) return
    try {
      const lastSeq = detailLogs.value.length ? detailLogs.value[detailLogs.value.length - 1].seq : 0
      const [fresh, logs] = await Promise.all([api.job(job.id), api.jobLogs(job.id, lastSeq)])
      detailJob.value = fresh.job
      if (logs.items.length) detailLogs.value = [...detailLogs.value, ...logs.items]
      if (fresh.job.status !== 'pending' && fresh.job.status !== 'running') {
        stopPolling()
        await load()
      }
    } catch {
      stopPolling()
    }
  }, 1800)
}

function stopPolling() {
  if (pollTimer) {
    window.clearInterval(pollTimer)
    pollTimer = 0
  }
}

async function cancelJob() {
  if (!detailJob.value) return
  actionBusy.value = true
  try {
    await api.cancelJob(detailJob.value.id)
    toast.add({ severity: 'success', summary: '任务已取消', life: 2200 })
    await openJob(detailJob.value.id)
    await load()
  } catch (reason) {
    toast.add({ severity: 'error', summary: '取消失败', detail: reason instanceof Error ? reason.message : '', life: 4000 })
  } finally {
    actionBusy.value = false
  }
}

async function retryJob() {
  if (!detailJob.value) return
  actionBusy.value = true
  try {
    const response = await api.retryJob(detailJob.value.id)
    toast.add({ severity: 'success', summary: `已创建重试任务 #${response.job.id}`, life: 2600 })
    await load()
  } catch (reason) {
    toast.add({ severity: 'error', summary: '重试失败', detail: reason instanceof Error ? reason.message : '', life: 4000 })
  } finally {
    actionBusy.value = false
  }
}

function rollback() {
  if (!detailJob.value) return
  confirm.require({
    header: '回滚到上一个版本',
    message: '主机会把最近一次备份恢复回来并重启服务。确定继续吗？',
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '回滚',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    accept: async () => {
      actionBusy.value = true
      try {
        const response = await api.rollbackJob(detailJob.value!.id)
        toast.add({ severity: 'success', summary: `已创建回滚任务 #${response.job.id}`, life: 2800 })
        await load()
        await openJob(response.job.id)
      } catch (reason) {
        toast.add({ severity: 'error', summary: '回滚失败', detail: reason instanceof Error ? reason.message : '', life: 4000 })
      } finally {
        actionBusy.value = false
      }
    },
  })
}

function closeDetail() {
  stopPolling()
  detailJob.value = null
  if (route.query.job) router.replace({ path: '/app/deployments' })
}

watch(
  () => [filters.project_id, filters.status],
  () => {
    page.value = 1
    load()
  },
)

watch(page, load)

let searchTimer = 0
watch(
  () => filters.q,
  () => {
    window.clearTimeout(searchTimer)
    searchTimer = window.setTimeout(() => {
      page.value = 1
      load()
    }, 280)
  },
)

watch(
  () => route.query.job,
  value => {
    const id = Number(value)
    if (id) openJob(id)
  },
)

watch(detailVisible, visible => {
  if (!visible) closeDetail()
})

onMounted(async () => {
  try {
    const response = await api.projects()
    projects.value = response.items
  } catch {
    projects.value = []
  }
  await load()
  const id = Number(route.query.job)
  if (id) openJob(id)
})

onBeforeUnmount(stopPolling)
</script>

<template>
  <section class="surface-panel mb-4 rounded-lg p-3">
    <div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-[1.4fr_1fr_1.4fr]">
      <Select v-model="filters.project_id" :options="projectOptions" option-label="label" option-value="value" placeholder="全部项目" show-clear filter fluid />
      <Select v-model="filters.status" :options="statusOptions" option-label="label" option-value="value" fluid />
      <InputText v-model="filters.q" placeholder="搜索版本号、发起人" fluid />
    </div>
  </section>

  <Message v-if="error" severity="error" :closable="false" class="mb-4">{{ error }}</Message>

  <section class="surface-panel rounded-lg">
    <DataTable
      :value="jobs"
      :loading="loading"
      data-key="id"
      size="small"
      class="text-sm"
      row-hover
      :paginator="total > pageSize"
      :rows="pageSize"
      :total-records="total"
      :first="(page - 1) * pageSize"
      :rows-per-page-options="[pageSize]"
      @page="event => (page = event.page + 1)"
      @row-click="event => openJob(event.data.id)"
    >
      <template #empty><div class="table-empty-state">还没有部署记录。上传版本并选择目标主机后，任务会出现在这里。</div></template>
      <Column header="#" class="hidden sm:table-cell" style="width: 5rem">
        <template #body="{ data: job }"><span class="mono text-xs text-muted-color">#{{ job.id }}</span></template>
      </Column>
      <Column header="版本" class="w-[30%] sm:w-[6.5rem]">
        <template #body="{ data: job }">
          <div class="flex items-center gap-2">
            <span class="mono text-xs font-semibold text-color">{{ job.version }}</span>
            <span v-if="job.mode === 'rollback'" class="rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">回滚</span>
          </div>
        </template>
      </Column>
      <Column header="主机" class="w-[40%] sm:w-[8rem]">
        <template #body="{ data: job }"><span class="truncate text-xs text-color">{{ job.target?.name || `#${job.target_id}` }}</span></template>
      </Column>
      <Column header="状态" class="w-[30%] sm:w-[7rem]">
        <template #body="{ data: job }"><StatusTag :status="job.status" /></template>
      </Column>
      <Column header="触发" class="hidden md:table-cell" style="width: 5rem">
        <template #body="{ data: job }"><span class="text-xs text-muted-color">{{ triggerLabel(job.trigger) }}</span></template>
      </Column>
      <Column header="发起人" class="hidden lg:table-cell" style="width: 8rem">
        <template #body="{ data: job }"><span class="truncate text-xs text-muted-color">{{ job.requested_by || '—' }}</span></template>
      </Column>
      <Column header="耗时" class="hidden sm:table-cell" style="width: 6rem">
        <template #body="{ data: job }"><span class="mono text-xs text-muted-color">{{ formatDuration(job.duration_ms) }}</span></template>
      </Column>
      <Column header="时间" class="hidden sm:table-cell" style="width: 9rem">
        <template #body="{ data: job }"><span class="text-xs text-muted-color">{{ formatDateTime(job.created_at) }}</span></template>
      </Column>
      <Column header="结果" class="hidden xl:table-cell">
        <template #body="{ data: job }">
          <span class="line-clamp-1 text-xs" :class="job.error ? 'text-red-600 dark:text-red-400' : 'text-muted-color'">{{ job.error || '—' }}</span>
        </template>
      </Column>
    </DataTable>
  </section>

  <Drawer v-model:visible="detailVisible" position="right" :style="{ width: '48rem', maxWidth: '96vw' }" @hide="closeDetail">
    <template #header>
      <div v-if="detailJob" class="flex min-w-0 items-center gap-2">
        <span class="mono text-sm font-semibold text-color">#{{ detailJob.id }}</span>
        <span class="mono text-sm text-color">{{ detailJob.version }}</span>
        <StatusTag :status="detailJob.status" />
        <span v-if="detailJob.mode === 'rollback'" class="shrink-0 whitespace-nowrap rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-semibold text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">回滚任务</span>
        <span v-else-if="detailJob.rolled_back" class="shrink-0 whitespace-nowrap rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-semibold text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">已自动回滚</span>
      </div>
    </template>

    <div v-if="detailJob" class="space-y-4">
      <div class="surface-panel rounded-lg p-3">
        <div class="detail-row">
          <span>项目</span>
          <strong>{{ detailProject?.name || `#${detailJob.project_id}` }}</strong>
        </div>
        <div class="detail-row">
          <span>目标主机</span>
          <strong>{{ detailJob.target?.name || `#${detailJob.target_id}` }}<span v-if="detailJob.target" class="ml-1 font-normal text-muted-color">· {{ platformLabel(detailJob.target.platform) }}</span></strong>
        </div>
        <div class="detail-row">
          <span>部署路径</span>
          <strong class="mono">{{ detailJob.deploy_dir || '（Agent 默认）' }}</strong>
        </div>
        <div class="detail-row">
          <span>触发方式</span>
          <strong>{{ triggerLabel(detailJob.trigger) }} · {{ detailJob.requested_by || '—' }}</strong>
        </div>
        <div class="detail-row">
          <span>开始 / 结束</span>
          <strong>{{ formatDateTime(detailJob.started_at) }} → {{ formatDateTime(detailJob.finished_at) }}</strong>
        </div>
        <div class="detail-row">
          <span>耗时</span>
          <strong>{{ formatDuration(detailJob.duration_ms) }}</strong>
        </div>
        <div v-if="detailJob.backup_path" class="detail-row">
          <span>备份位置</span>
          <strong class="mono">{{ detailJob.backup_path }}</strong>
        </div>
        <div v-if="detailJob.message" class="detail-row">
          <span>说明</span>
          <strong>{{ detailJob.message }}</strong>
        </div>
      </div>

      <Message v-if="detailJob.error" severity="error" :closable="false">{{ detailJob.error }}</Message>

      <div class="flex flex-wrap items-center gap-2">
        <Button v-if="detailJob.status === 'pending'" severity="secondary" size="small" :loading="actionBusy" @click="cancelJob"><i class="pi pi-ban"></i> 取消排队</Button>
        <Button severity="secondary" size="small" :loading="actionBusy" @click="retryJob"><Replay :size="15" /> 重新部署</Button>
        <Button severity="danger" outlined size="small" :loading="actionBusy" :disabled="isActive" @click="rollback"><i class="pi pi-history"></i> 回滚到上一版本</Button>
        <span v-if="isActive" class="text-xs text-muted-color"><Bolt :size="12" class="mr-1 inline" />正在实时刷新日志…</span>
      </div>

      <div>
        <div class="mb-2 flex items-center justify-between">
          <h3 class="text-sm font-semibold text-color">执行日志</h3>
          <span class="text-xs text-muted-color">{{ detailLogs.length }} 行</span>
        </div>
        <LogViewer :logs="detailLogs" />
      </div>
    </div>
  </Drawer>
</template>

<style scoped>
.detail-row {
  display: grid;
  grid-template-columns: minmax(6rem, 0.5fr) minmax(0, 1.5fr);
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
