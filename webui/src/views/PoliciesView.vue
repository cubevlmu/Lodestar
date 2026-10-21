<script setup lang="ts">
import Plus from '@primeicons/vue/plus'
import Trash from '@primeicons/vue/trash'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import { computed, onMounted, reactive, ref } from 'vue'
import { api } from '../services/api'
import type { BlockedVersion, Project, VersionPolicy } from '../types'
import { codeToVersion, formatDateTime, platformLabel } from '../utils'

const toast = useToast()
const confirm = useConfirm()

const projects = ref<Project[]>([])
const policies = ref<VersionPolicy[]>([])
const blocked = ref<BlockedVersion[]>([])
const loading = ref(true)
const error = ref('')
const tab = ref<'policy' | 'blocked'>('policy')

const projectOptions = computed(() => projects.value.map(project => ({ label: `${project.name} · ${project.app_id}`, value: project.id })))
const projectNames = computed(() => {
  const map = new Map<number, string>()
  for (const project of projects.value) map.set(project.id, project.name)
  return map
})
const platformOptions = [
  { label: 'Linux', value: 'linux' },
  { label: 'Windows', value: 'windows' },
  { label: 'macOS', value: 'darwin' },
  { label: 'Android', value: 'android' },
  { label: 'iOS', value: 'ios' },
]
const channelOptions = [
  { label: 'stable', value: 'stable' },
  { label: 'beta', value: 'beta' },
  { label: 'dev', value: 'dev' },
]

const policyVisible = ref(false)
const policySaving = ref(false)
const policyError = ref('')
const policyEditing = ref<VersionPolicy | null>(null)
const policyForm = reactive({
  project_id: null as number | null,
  platform: 'linux',
  channel: 'stable',
  min_allowed_version: '',
  message: '',
  maintenance: false,
  maintenance_msg: '',
})

const blockedVisible = ref(false)
const blockedSaving = ref(false)
const blockedError = ref('')
const blockedForm = reactive({
  project_id: null as number | null,
  platform: 'linux',
  channel: 'stable',
  version: '',
  reason: '',
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [policyResponse, blockedResponse] = await Promise.all([api.policies(), api.blockedVersions()])
    policies.value = policyResponse.items
    blocked.value = blockedResponse.items
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '加载失败'
  } finally {
    loading.value = false
  }
}

function openPolicy(policy?: VersionPolicy) {
  policyError.value = ''
  policyEditing.value = policy ?? null
  Object.assign(policyForm, {
    project_id: policy?.project_id ?? projects.value[0]?.id ?? null,
    platform: policy?.platform ?? 'linux',
    channel: policy?.channel ?? 'stable',
    min_allowed_version: policy ? codeToVersion(policy.min_allowed_code) : '',
    message: policy?.message ?? '',
    maintenance: policy?.maintenance ?? false,
    maintenance_msg: policy?.maintenance_msg ?? '',
  })
  if (policyForm.min_allowed_version === '—') policyForm.min_allowed_version = ''
  policyVisible.value = true
}

async function savePolicy() {
  policyError.value = ''
  if (!policyForm.project_id) {
    policyError.value = '请选择项目。'
    return
  }
  policySaving.value = true
  try {
    await api.savePolicy({
      project_id: policyForm.project_id,
      platform: policyForm.platform,
      channel: policyForm.channel,
      min_allowed_version: policyForm.min_allowed_version.trim(),
      message: policyForm.message,
      maintenance: policyForm.maintenance,
      maintenance_msg: policyForm.maintenance_msg,
    })
    toast.add({ severity: 'success', summary: '策略已保存', life: 2200 })
    policyVisible.value = false
    await load()
  } catch (reason) {
    policyError.value = reason instanceof Error ? reason.message : '保存失败'
  } finally {
    policySaving.value = false
  }
}

function removePolicy(policy: VersionPolicy) {
  confirm.require({
    header: '删除策略',
    message: `删除 ${platformLabel(policy.platform)} / ${policy.channel} 的版本策略？客户端将不再受最低版本与维护模式限制。`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    accept: async () => {
      try {
        await api.deletePolicy(policy.id)
        toast.add({ severity: 'success', summary: '策略已删除', life: 2200 })
        await load()
      } catch (reason) {
        toast.add({ severity: 'error', summary: '删除失败', detail: reason instanceof Error ? reason.message : '', life: 4000 })
      }
    },
  })
}

function openBlocked() {
  blockedError.value = ''
  Object.assign(blockedForm, {
    project_id: projects.value[0]?.id ?? null,
    platform: 'linux',
    channel: 'stable',
    version: '',
    reason: '',
  })
  blockedVisible.value = true
}

async function saveBlocked() {
  blockedError.value = ''
  if (!blockedForm.project_id) {
    blockedError.value = '请选择项目。'
    return
  }
  if (!/^\d+(\.\d+){0,2}$/.test(blockedForm.version.trim())) {
    blockedError.value = '版本号格式应为 1.2.3。'
    return
  }
  blockedSaving.value = true
  try {
    await api.createBlocked({ ...blockedForm, version: blockedForm.version.trim() })
    toast.add({ severity: 'success', summary: '版本已封禁', life: 2200 })
    blockedVisible.value = false
    await load()
  } catch (reason) {
    blockedError.value = reason instanceof Error ? reason.message : '保存失败'
  } finally {
    blockedSaving.value = false
  }
}

function removeBlocked(item: BlockedVersion) {
  confirm.require({
    header: '解除封禁',
    message: `确定解除 ${codeToVersion(item.version_code)} 的封禁吗？`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '解除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    accept: async () => {
      try {
        await api.deleteBlocked(item.id)
        toast.add({ severity: 'success', summary: '已解除封禁', life: 2200 })
        await load()
      } catch (reason) {
        toast.add({ severity: 'error', summary: '操作失败', detail: reason instanceof Error ? reason.message : '', life: 4000 })
      }
    },
  })
}

async function toggleBlocked(item: BlockedVersion) {
  try {
    await api.updateBlocked(item.id, { enabled: !item.enabled })
    await load()
  } catch (reason) {
    toast.add({ severity: 'error', summary: '更新失败', detail: reason instanceof Error ? reason.message : '', life: 4000 })
  }
}

onMounted(async () => {
  try {
    const response = await api.projects()
    projects.value = response.items
  } catch {
    projects.value = []
  }
  await load()
})
</script>

<template>
  <Teleport to="#page-actions" defer>
    <Button v-if="tab === 'policy'" size="small" @click="openPolicy()"><Plus :size="16" /> 新建策略</Button>
    <Button v-else size="small" @click="openBlocked"><Plus :size="16" /> 封禁版本</Button>
  </Teleport>

  <Message v-if="error" severity="error" :closable="false" class="mb-4">{{ error }}</Message>

  <Tabs v-model:value="tab">
    <TabList>
      <Tab value="policy">版本策略</Tab>
      <Tab value="blocked">版本封禁</Tab>
    </TabList>
    <TabPanels>
      <TabPanel value="policy">
        <p class="mb-3 text-xs text-muted-color">
          客户端调用 <span class="mono">/api/v1/check</span> 时按平台与通道匹配策略：低于最低版本会被拒绝，开启维护模式会阻止所有请求。
        </p>
        <section class="surface-panel rounded-lg">
          <DataTable :value="policies" :loading="loading" data-key="id" size="small" class="text-sm">
            <template #empty><div class="table-empty-state">还没有版本策略。默认允许所有版本更新。</div></template>
            <Column header="项目" style="width: min(12rem, 34vw)">
              <template #body="{ data: policy }"><span class="truncate text-xs text-color">{{ projectNames.get(policy.project_id) || `#${policy.project_id}` }}</span></template>
            </Column>
            <Column header="平台" class="hidden sm:table-cell" style="width: 8rem">
              <template #body="{ data: policy }"><span class="text-xs text-color">{{ platformLabel(policy.platform) }}</span></template>
            </Column>
            <Column header="通道" class="hidden md:table-cell" style="width: 6rem">
              <template #body="{ data: policy }"><span class="mono text-xs text-muted-color">{{ policy.channel }}</span></template>
            </Column>
            <Column header="最低版本" style="width: 5.5rem">
              <template #body="{ data: policy }">
                <span class="mono text-xs text-color">{{ policy.min_allowed_code ? codeToVersion(policy.min_allowed_code) : '不限' }}</span>
              </template>
            </Column>
            <Column header="维护模式" class="hidden sm:table-cell" style="width: 7rem">
              <template #body="{ data: policy }">
                <span class="text-xs" :class="policy.maintenance ? 'font-semibold text-red-600 dark:text-red-400' : 'text-muted-color'">{{ policy.maintenance ? '已开启' : '关闭' }}</span>
              </template>
            </Column>
            <Column header="提示文案" class="hidden lg:table-cell">
              <template #body="{ data: policy }"><span class="line-clamp-1 text-xs text-muted-color">{{ policy.maintenance ? policy.maintenance_msg : policy.message || '—' }}</span></template>
            </Column>
            <Column header="操作" class="w-[4rem] sm:w-[6rem]">
              <template #body="{ data: policy }">
                <div class="flex items-center gap-1">
                  <Button severity="secondary" text size="small" class="!h-7 !w-7 !p-0" @click="openPolicy(policy)"><i class="pi pi-pen-to-square text-[13px]"></i></Button>
                  <Button severity="danger" text size="small" class="!h-7 !w-7 !p-0" @click="removePolicy(policy)"><Trash :size="15" /></Button>
                </div>
              </template>
            </Column>
          </DataTable>
        </section>
      </TabPanel>

      <TabPanel value="blocked">
        <p class="mb-3 text-xs text-muted-color">
          封禁指定版本号：客户端当前版本命中封禁时会收到 <span class="mono">version_blocked</span>，通常配合强制更新使用。
        </p>
        <section class="surface-panel rounded-lg">
          <DataTable :value="blocked" :loading="loading" data-key="id" size="small" class="text-sm">
            <template #empty><div class="table-empty-state">没有封禁的版本。</div></template>
            <Column header="项目" style="width: min(12rem, 34vw)">
              <template #body="{ data: item }"><span class="truncate text-xs text-color">{{ projectNames.get(item.project_id) || `#${item.project_id}` }}</span></template>
            </Column>
            <Column header="平台" class="hidden sm:table-cell" style="width: 8rem">
              <template #body="{ data: item }"><span class="text-xs text-color">{{ platformLabel(item.platform) }}</span></template>
            </Column>
            <Column header="通道" class="hidden md:table-cell" style="width: 6rem">
              <template #body="{ data: item }"><span class="mono text-xs text-muted-color">{{ item.channel }}</span></template>
            </Column>
            <Column header="版本" style="width: 6rem">
              <template #body="{ data: item }"><span class="mono text-xs text-color">{{ codeToVersion(item.version_code) }}</span></template>
            </Column>
            <Column header="原因" class="hidden lg:table-cell">
              <template #body="{ data: item }"><span class="line-clamp-1 text-xs text-muted-color">{{ item.reason || '—' }}</span></template>
            </Column>
            <Column header="状态" class="hidden sm:table-cell" style="width: 6rem">
              <template #body="{ data: item }">
                <ToggleSwitch :model-value="item.enabled" @update:model-value="toggleBlocked(item)" />
              </template>
            </Column>
            <Column header="创建时间" class="hidden xl:table-cell" style="width: 9rem">
              <template #body="{ data: item }"><span class="text-xs text-muted-color">{{ formatDateTime(item.created_at) }}</span></template>
            </Column>
            <Column header="操作" class="w-[4rem]">
              <template #body="{ data: item }">
                <Button severity="danger" text size="small" class="!h-7 !w-7 !p-0" @click="removeBlocked(item)"><Trash :size="15" /></Button>
              </template>
            </Column>
          </DataTable>
        </section>
      </TabPanel>
    </TabPanels>
  </Tabs>

  <Dialog v-model:visible="policyVisible" modal :header="policyEditing ? '编辑版本策略' : '新建版本策略'" :style="{ width: '34rem', maxWidth: '94vw' }">
    <div class="flex flex-col gap-4">
      <div>
        <label class="field-label">项目</label>
        <Select v-model="policyForm.project_id" :options="projectOptions" option-label="label" option-value="value" placeholder="选择项目" filter fluid />
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="field-label">平台</label>
          <Select v-model="policyForm.platform" :options="platformOptions" option-label="label" option-value="value" fluid />
        </div>
        <div>
          <label class="field-label">通道</label>
          <Select v-model="policyForm.channel" :options="channelOptions" option-label="label" option-value="value" fluid />
        </div>
      </div>
      <div>
        <label class="field-label">最低允许版本</label>
        <InputText v-model="policyForm.min_allowed_version" placeholder="例如 1.2.0，留空表示不限制" fluid />
      </div>
      <div>
        <label class="field-label">版本过低提示</label>
        <InputText v-model="policyForm.message" placeholder="版本过低，请更新后继续使用。" fluid />
      </div>
      <label class="flex cursor-pointer items-center justify-between gap-3 rounded-md border border-surface-200 px-3 py-2.5 dark:border-surface-800">
        <span class="text-sm text-color">维护模式</span>
        <ToggleSwitch v-model="policyForm.maintenance" />
      </label>
      <div v-if="policyForm.maintenance">
        <label class="field-label">维护提示</label>
        <InputText v-model="policyForm.maintenance_msg" placeholder="服务正在维护，请稍后再试。" fluid />
      </div>
      <Message v-if="policyError" severity="error" size="small">{{ policyError }}</Message>
    </div>
    <template #footer>
      <Button severity="secondary" text @click="policyVisible = false">取消</Button>
      <Button :loading="policySaving" @click="savePolicy">保存</Button>
    </template>
  </Dialog>

  <Dialog v-model:visible="blockedVisible" modal header="封禁版本" :style="{ width: '32rem', maxWidth: '94vw' }">
    <div class="flex flex-col gap-4">
      <div>
        <label class="field-label">项目</label>
        <Select v-model="blockedForm.project_id" :options="projectOptions" option-label="label" option-value="value" placeholder="选择项目" filter fluid />
      </div>
      <div class="grid grid-cols-3 gap-3">
        <div>
          <label class="field-label">平台</label>
          <Select v-model="blockedForm.platform" :options="platformOptions" option-label="label" option-value="value" fluid />
        </div>
        <div>
          <label class="field-label">通道</label>
          <Select v-model="blockedForm.channel" :options="channelOptions" option-label="label" option-value="value" fluid />
        </div>
        <div>
          <label class="field-label">版本</label>
          <InputText v-model="blockedForm.version" placeholder="1.0.0" fluid />
        </div>
      </div>
      <div>
        <label class="field-label">封禁原因</label>
        <Textarea v-model="blockedForm.reason" rows="3" auto-resize placeholder="该版本存在严重问题，请更新后继续使用。" fluid />
      </div>
      <Message v-if="blockedError" severity="error" size="small">{{ blockedError }}</Message>
    </div>
    <template #footer>
      <Button severity="secondary" text @click="blockedVisible = false">取消</Button>
      <Button :loading="blockedSaving" @click="saveBlocked">封禁</Button>
    </template>
  </Dialog>
</template>
