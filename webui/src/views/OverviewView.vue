<script setup lang="ts">
import Bolt from '@primeicons/vue/bolt'
import Box from '@primeicons/vue/box'
import ChartLine from '@primeicons/vue/chart-line'
import Server from '@primeicons/vue/server'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Message from 'primevue/message'
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import AppPageSkeleton from '../components/AppPageSkeleton.vue'
import StatusTag from '../components/StatusTag.vue'
import { api } from '../services/api'
import type { Overview } from '../types'
import { formatBytes, formatDateTime, formatDuration, formatRelative, formatUptime, formatVersion, platformLabel, triggerLabel } from '../utils'

const router = useRouter()
const data = ref<Overview | null>(null)
const loading = ref(true)
const error = ref('')

const maxDaily = computed(() => Math.max(1, ...(data.value?.daily ?? []).map(day => day.total)))
const successRate = computed(() => {
  const rate = data.value?.success_rate ?? 0
  return `${rate.toFixed(rate >= 10 ? 0 : 1)}%`
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api.overview()
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '加载失败'
  } finally {
    loading.value = false
  }
}

function openJob(id: number) {
  router.push({ path: '/app/deployments', query: { job: String(id) } })
}

onMounted(load)
</script>

<template>
  <AppPageSkeleton v-if="loading" />
  <Message v-else-if="error" severity="error" :closable="false">{{ error }}</Message>

  <div v-else-if="data" class="space-y-4">
    <div class="grid grid-cols-2 gap-3 xl:grid-cols-4">
      <article class="metric-card">
        <div class="metric-label"><Box :size="14" /> 项目</div>
        <div class="metric-value">{{ data.projects }}</div>
        <div class="metric-hint">{{ data.releases }} 个版本包 · 本周 +{{ data.releases_week }}</div>
      </article>
      <article class="metric-card">
        <div class="metric-label"><Server :size="14" /> 生产主机</div>
        <div class="metric-value">{{ data.targets_online }}<span class="metric-suffix">/ {{ data.targets }}</span></div>
        <div class="metric-hint">{{ data.targets_online === data.targets && data.targets > 0 ? '全部在线' : '在线 / 已登记' }}</div>
      </article>
      <article class="metric-card">
        <div class="metric-label"><Bolt :size="14" /> 今日部署</div>
        <div class="metric-value">{{ data.jobs_today }}</div>
        <div class="metric-hint">本周 {{ data.jobs_week }} 次 · 平均 {{ formatDuration(data.avg_duration_ms) }}</div>
      </article>
      <article class="metric-card">
        <div class="metric-label"><ChartLine :size="14" /> 近 7 天成功率</div>
        <div class="metric-value">{{ successRate }}</div>
        <div class="metric-hint">成功 {{ data.jobs_success_week }} · 失败 {{ data.jobs_failed_week }}</div>
      </article>
    </div>

    <div class="grid min-w-0 gap-4 xl:grid-cols-[1.55fr_1fr]">
      <section class="surface-panel min-w-0 rounded-lg p-4 sm:p-5">
        <div class="flex items-start justify-between gap-3">
          <div>
            <h2 class="text-sm font-semibold text-color">部署趋势</h2>
            <p class="mt-0.5 text-xs text-muted-color">最近 14 天的部署次数，按结果拆分。</p>
          </div>
          <div class="flex items-center gap-3 text-[11px] text-muted-color">
            <span class="flex items-center gap-1"><i class="status-dot" style="background: var(--p-emerald-500)"></i>成功</span>
            <span class="flex items-center gap-1"><i class="status-dot" style="background: var(--p-red-500)"></i>失败</span>
          </div>
        </div>

        <div class="mt-5 flex gap-1.5 sm:gap-2.5">
          <div v-for="day in data.daily" :key="day.day" class="spark-item flex min-w-0 flex-1 flex-col">
            <div class="spark-row">
              <div class="spark-col" :title="`${day.day}：${day.total} 次（成功 ${day.success} / 失败 ${day.failed}）`">
                <div
                  v-if="day.failed"
                  class="spark-seg"
                  :style="{ height: `${(day.failed / maxDaily) * 100}%`, background: 'var(--p-red-500)' }"
                ></div>
                <div
                  v-if="day.success"
                  class="spark-seg"
                  :style="{ height: `${(day.success / maxDaily) * 100}%`, background: 'var(--p-emerald-500)' }"
                ></div>
                <div
                  v-if="!day.total"
                  class="w-full rounded-sm"
                  style="height: 3px; background: var(--p-content-border-color)"
                ></div>
              </div>
            </div>
            <div class="spark-day">{{ day.day.slice(5) }}</div>
          </div>
        </div>

        <div class="mt-5 grid grid-cols-3 gap-3 border-t border-surface-200 pt-4 text-xs dark:border-surface-800">
          <div>
            <div class="text-muted-color">产物占用</div>
            <div class="mt-1 text-sm font-semibold text-color">{{ formatBytes(data.storage_bytes) }}</div>
          </div>
          <div>
            <div class="text-muted-color">运行时长</div>
            <div class="mt-1 text-sm font-semibold text-color">{{ formatUptime(data.server.uptime) }}</div>
          </div>
          <div>
            <div class="text-muted-color">服务版本</div>
            <div class="mt-1 text-sm font-semibold text-color mono">{{ formatVersion(data.server.version) }}</div>
          </div>
        </div>
      </section>

      <section class="surface-panel flex min-w-0 flex-col rounded-lg p-4 sm:p-5">
        <div class="flex items-center justify-between gap-2">
          <h2 class="text-sm font-semibold text-color">生产主机</h2>
          <RouterLink to="/app/targets" class="text-xs font-medium text-primary hover:underline">管理</RouterLink>
        </div>
        <div v-if="!data.targets_list.length" class="table-empty-state !min-h-[10rem]">
          还没有登记生产主机。先在「目标主机」中添加，并在服务器上启动 Agent。
        </div>
        <ul v-else class="mt-3 divide-y divide-surface-200 dark:divide-surface-800">
          <li v-for="target in data.targets_list" :key="target.id" class="flex items-center gap-3 py-2.5">
            <i class="status-dot shrink-0" :style="{ background: target.status === 'online' ? 'var(--p-emerald-500)' : target.status === 'deploying' ? 'var(--p-blue-500)' : target.status === 'error' ? 'var(--p-red-500)' : 'var(--p-surface-400)' }"></i>
            <div class="min-w-0 flex-1">
              <div class="truncate text-sm font-medium text-color">{{ target.name }}</div>
              <div class="truncate text-xs text-muted-color">
                {{ platformLabel(target.platform) }} · {{ target.current_version || '未部署' }} · {{ formatRelative(target.last_seen_at) }}
              </div>
            </div>
            <StatusTag :status="target.status" kind="target" />
          </li>
        </ul>
      </section>
    </div>

    <section class="surface-panel rounded-lg">
      <div class="flex items-center justify-between gap-2 border-b border-surface-200 px-4 py-3 dark:border-surface-800">
        <h2 class="text-sm font-semibold text-color">最近部署</h2>
        <RouterLink to="/app/deployments" class="text-xs font-medium text-primary hover:underline">查看全部</RouterLink>
      </div>
      <DataTable :value="data.recent_jobs" data-key="id" size="small" row-hover class="text-sm" @row-click="event => openJob(event.data.id)">
        <template #empty><div class="table-empty-state">还没有部署记录。</div></template>
        <Column header="#" class="hidden sm:table-cell" style="width: 5rem">
          <template #body="{ data: job }"><span class="mono text-xs text-muted-color">#{{ job.id }}</span></template>
        </Column>
        <Column header="版本">
          <template #body="{ data: job }">
            <div class="flex items-center gap-2">
              <span class="mono text-xs font-semibold text-color">{{ job.version }}</span>
              <span v-if="job.mode === 'rollback'" class="rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">回滚</span>
            </div>
          </template>
        </Column>
        <Column header="主机">
          <template #body="{ data: job }">
            <span class="text-xs text-color">{{ job.target?.name || `#${job.target_id}` }}</span>
          </template>
        </Column>
        <Column header="状态" style="width: 7rem">
          <template #body="{ data: job }"><StatusTag :status="job.status" /></template>
        </Column>
        <Column header="触发" class="hidden md:table-cell" style="width: 5rem">
          <template #body="{ data: job }"><span class="text-xs text-muted-color">{{ triggerLabel(job.trigger) }}</span></template>
        </Column>
        <Column header="耗时" class="hidden sm:table-cell" style="width: 6rem">
          <template #body="{ data: job }"><span class="mono text-xs text-muted-color">{{ formatDuration(job.duration_ms) }}</span></template>
        </Column>
        <Column header="时间" class="hidden sm:table-cell" style="width: 9rem">
          <template #body="{ data: job }"><span class="text-xs text-muted-color">{{ formatDateTime(job.created_at) }}</span></template>
        </Column>
      </DataTable>
    </section>
  </div>
</template>
