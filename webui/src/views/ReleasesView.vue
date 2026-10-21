<script setup lang="ts">
import Bolt from '@primeicons/vue/bolt'
import CloudUpload from '@primeicons/vue/cloud-upload'
import Plus from '@primeicons/vue/plus'
import Trash from '@primeicons/vue/trash'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import Tag from 'primevue/tag'
import { api } from '../services/api'
import type { DeployConfig, Project, Release, Target } from '../types'
import { formatBytes, formatDateTime, platformLabel } from '../utils'

const toast = useToast()
const confirm = useConfirm()

const projects = ref<Project[]>([])
const targets = ref<Target[]>([])
const releases = ref<Release[]>([])
const total = ref(0)
const loading = ref(true)
const error = ref('')

const page = ref(1)
const pageSize = 20
const filters = reactive<{ project_id: number | null; platform: string; channel: string; q: string }>({
  project_id: null,
  platform: '',
  channel: '',
  q: '',
})

const projectOptions = computed(() => projects.value.map(project => ({ label: `${project.name} · ${project.app_id}`, value: project.id })))
const platformOptions = [
  { label: '全部平台', value: '' },
  { label: 'Linux', value: 'linux' },
  { label: 'Windows', value: 'windows' },
  { label: 'macOS', value: 'darwin' },
  { label: 'Android', value: 'android' },
  { label: 'iOS', value: 'ios' },
]
const channelOptions = [
  { label: '全部通道', value: '' },
  { label: 'stable', value: 'stable' },
  { label: 'beta', value: 'beta' },
  { label: 'dev', value: 'dev' },
]
const uploadPlatformOptions = platformOptions.slice(1)
const uploadChannelOptions = channelOptions.slice(1)

const projectNames = computed(() => {
  const map = new Map<number, string>()
  for (const project of projects.value) map.set(project.id, project.name)
  return map
})

// ---------- upload ----------
const uploadVisible = ref(false)
const uploading = ref(false)
const uploadError = ref('')
const fileInput = ref<HTMLInputElement | null>(null)
const upload = reactive({
  project_id: null as number | null,
  version: '',
  platform: 'linux',
  arch: 'amd64',
  channel: 'stable',
  title: '',
  changelog: '',
  force: false,
  enabled: true,
  rollout_percent: 100,
  deploy: true,
  target_ids: [] as number[],
  file: null as File | null,
})

// A host can serve any project; only the platform/channel have to match.
const uploadTargets = computed(() =>
  targets.value.filter(target => target.platform === upload.platform && target.channel === upload.channel),
)

// ---------- deploy existing release ----------
const deployVisible = ref(false)
const deploying = ref(false)
const deployError = ref('')
const deployReleaseTarget = ref<Release | null>(null)
const deployConfigs = ref<DeployConfig[]>([])
const deployConfigIDs = ref<number[]>([])

// A deployment always goes through a saved 部署配置: it already carries the
// install path, restart command and whitelist, so nothing is typed here.
const deployCandidates = computed(() => {
  const release = deployReleaseTarget.value
  if (!release) return []
  return deployConfigs.value.filter(item => item.target_platform === release.platform)
})

// ---------- edit ----------
const editVisible = ref(false)
const editSaving = ref(false)
const editError = ref('')
const editRelease = ref<Release | null>(null)
const editForm = reactive({ title: '', changelog: '', enabled: true, force: false, rollout_percent: 100, min_supported_code: 0 })

async function load() {
  loading.value = true
  error.value = ''
  try {
    const response = await api.releases({
      project_id: filters.project_id ?? undefined,
      platform: filters.platform || undefined,
      channel: filters.channel || undefined,
      q: filters.q || undefined,
      page: page.value,
      size: pageSize,
    })
    releases.value = response.items
    total.value = response.total
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function loadTargets() {
  try {
    const response = await api.targets()
    targets.value = response.items
  } catch {
    targets.value = []
  }
}

function openUpload() {
  uploadError.value = ''
  Object.assign(upload, {
    project_id: filters.project_id ?? projects.value[0]?.id ?? null,
    version: '',
    platform: 'linux',
    arch: 'amd64',
    channel: 'stable',
    title: '',
    changelog: '',
    force: false,
    enabled: true,
    rollout_percent: 100,
    deploy: true,
    target_ids: [],
    file: null,
  })
  if (fileInput.value) fileInput.value.value = ''
  uploadVisible.value = true
}

function pickFile(event: Event) {
  const input = event.target as HTMLInputElement
  upload.file = input.files?.[0] ?? null
}

async function submitUpload() {
  uploadError.value = ''
  if (!upload.project_id) {
    uploadError.value = '请选择项目。'
    return
  }
  if (!/^\d+(\.\d+){0,2}([-+].*)?$/.test(upload.version.trim())) {
    uploadError.value = '版本号格式应为 1.2.3。'
    return
  }
  if (!upload.file) {
    uploadError.value = '请选择要上传的构建产物。'
    return
  }
  uploading.value = true
  try {
    const form = new FormData()
    form.set('project_id', String(upload.project_id))
    form.set('version', upload.version.trim())
    form.set('platform', upload.platform)
    form.set('arch', upload.arch)
    form.set('channel', upload.channel)
    form.set('title', upload.title)
    form.set('changelog', upload.changelog)
    form.set('force', String(upload.force))
    form.set('enabled', String(upload.enabled))
    form.set('rollout_percent', String(upload.rollout_percent))
    form.set('package', upload.file)
    if (upload.deploy) {
      form.set('deploy', 'true')
      if (upload.target_ids.length) form.set('target_ids', upload.target_ids.join(','))
    }
    const response = await api.createRelease(form)
    const queued = response.jobs?.length ?? 0
    toast.add({
      severity: 'success',
      summary: `版本 ${response.release.version} 已登记`,
      detail: queued ? `已排队 ${queued} 个部署任务。` : '未触发部署。',
      life: 3600,
    })
    uploadVisible.value = false
    await load()
  } catch (reason) {
    uploadError.value = reason instanceof Error ? reason.message : '上传失败'
  } finally {
    uploading.value = false
  }
}

async function openDeploy(release: Release) {
  deployReleaseTarget.value = release
  deployError.value = ''
  deployConfigs.value = []
  deployConfigIDs.value = []
  deployVisible.value = true
  try {
    const response = await api.deployConfigs({ project_id: release.project_id })
    deployConfigs.value = response.items
    deployConfigIDs.value = deployCandidates.value.map(item => item.id)
  } catch (reason) {
    deployError.value = reason instanceof Error ? reason.message : '加载部署配置失败'
  }
}

async function submitDeploy() {
  if (!deployReleaseTarget.value) return
  if (!deployConfigIDs.value.length) {
    deployError.value = '请至少选择一个部署配置。'
    return
  }
  deploying.value = true
  deployError.value = ''
  try {
    // Only the host is sent: path and commands come from the saved config.
    const targets = deployConfigs.value
      .filter(item => deployConfigIDs.value.includes(item.id))
      .map(item => ({ target_id: item.target_id }))
    const response = await api.deployRelease(deployReleaseTarget.value.id, targets)
    toast.add({ severity: 'success', summary: `已排队 ${response.count} 个部署任务`, life: 3000 })
    deployVisible.value = false
  } catch (reason) {
    deployError.value = reason instanceof Error ? reason.message : '部署失败'
  } finally {
    deploying.value = false
  }
}

function openEdit(release: Release) {
  editRelease.value = release
  editError.value = ''
  Object.assign(editForm, {
    title: release.title,
    changelog: release.changelog,
    enabled: release.enabled,
    force: release.force,
    rollout_percent: release.rollout_percent,
    min_supported_code: release.min_supported_code,
  })
  editVisible.value = true
}

async function submitEdit() {
  if (!editRelease.value) return
  editSaving.value = true
  editError.value = ''
  try {
    await api.updateRelease(editRelease.value.id, { ...editForm })
    toast.add({ severity: 'success', summary: '版本已更新', life: 2200 })
    editVisible.value = false
    await load()
  } catch (reason) {
    editError.value = reason instanceof Error ? reason.message : '保存失败'
  } finally {
    editSaving.value = false
  }
}

function remove(release: Release) {
  confirm.require({
    header: '删除版本',
    message: `确定删除 ${release.version}（${platformLabel(release.platform)} / ${release.channel}）吗？已上传的产物文件也会删除。`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    accept: async () => {
      try {
        await api.deleteRelease(release.id)
        toast.add({ severity: 'success', summary: '版本已删除', life: 2200 })
        await load()
      } catch (reason) {
        toast.add({ severity: 'error', summary: '删除失败', detail: reason instanceof Error ? reason.message : '', life: 4000 })
      }
    },
  })
}

watch(
  () => [filters.project_id, filters.platform, filters.channel],
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

onMounted(async () => {
  try {
    const response = await api.projects()
    projects.value = response.items
  } catch {
    projects.value = []
  }
  await Promise.all([load(), loadTargets()])
})
</script>

<template>
  <Teleport to="#page-actions" defer>
    <Button size="small" @click="openUpload"><Plus :size="16" /> 上传版本</Button>
  </Teleport>

  <section class="surface-panel mb-4 rounded-lg p-3">
    <div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-[1.4fr_1fr_1fr_1.4fr]">
      <Select v-model="filters.project_id" :options="projectOptions" option-label="label" option-value="value" placeholder="全部项目" show-clear filter fluid />
      <Select v-model="filters.platform" :options="platformOptions" option-label="label" option-value="value" fluid />
      <Select v-model="filters.channel" :options="channelOptions" option-label="label" option-value="value" fluid />
      <InputText v-model="filters.q" placeholder="搜索版本号或更新说明" fluid />
    </div>
  </section>

  <Message v-if="error" severity="error" :closable="false" class="mb-4">{{ error }}</Message>

  <section class="surface-panel rounded-lg">
    <DataTable :value="releases" :loading="loading" data-key="id" size="small" row-hover class="text-sm" :paginator="total > pageSize" :rows="pageSize" :total-records="total" :first="(page - 1) * pageSize" :rows-per-page-options="[pageSize]" @page="event => (page = event.page + 1)">
      <template #empty><div class="table-empty-state">还没有版本。上传构建产物，或让 CI 调用发布接口。</div></template>
      <Column header="版本">
        <template #body="{ data: release }">
          <div class="flex min-w-0 flex-col">
            <div class="flex items-center gap-2">
              <span class="mono text-sm font-semibold text-color">{{ release.version }}</span>
              <span v-if="release.force" class="shrink-0 whitespace-nowrap rounded bg-red-100 px-1.5 py-0.5 text-[10px] font-semibold text-red-700 dark:bg-red-900/40 dark:text-red-300">强制</span>
              <span v-if="release.source === 'ci'" class="shrink-0 whitespace-nowrap rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-semibold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">CI</span>
            </div>
            <span class="truncate text-xs text-muted-color">{{ release.title || release.package?.file_name || '—' }}</span>
          </div>
        </template>
      </Column>
      <Column header="项目" class="hidden sm:table-cell" style="width: 11rem">
        <template #body="{ data: release }">
          <span class="truncate text-xs text-color">{{ projectNames.get(release.project_id) || `#${release.project_id}` }}</span>
        </template>
      </Column>
      <Column header="平台" class="hidden sm:table-cell" style="width: 9rem">
        <template #body="{ data: release }">
          <span class="text-xs text-color">{{ platformLabel(release.platform) }}</span>
          <span class="ml-1 text-[11px] text-muted-color mono">{{ release.arch }}</span>
        </template>
      </Column>
      <Column header="通道" class="hidden md:table-cell" style="width: 6rem">
        <template #body="{ data: release }"><span class="mono text-xs text-muted-color">{{ release.channel }}</span></template>
      </Column>
      <Column header="大小" class="hidden lg:table-cell" style="width: 6rem">
        <template #body="{ data: release }"><span class="mono text-xs text-muted-color">{{ formatBytes(release.package?.size || 0) }}</span></template>
      </Column>
      <Column header="灰度" class="hidden lg:table-cell" style="width: 5.5rem">
        <template #body="{ data: release }"><span class="mono text-xs text-color">{{ release.rollout_percent }}%</span></template>
      </Column>
      <Column header="状态" style="width: 6rem">
        <template #body="{ data: release }"><Tag :value="release.enabled ? '启用' : '停用'" :severity="release.enabled ? 'success' : 'secondary'" class="!px-2 !py-0.5 !text-xs" /></template>
      </Column>
      <Column header="创建时间" class="hidden xl:table-cell" style="width: 9rem">
        <template #body="{ data: release }"><span class="text-xs text-muted-color">{{ formatDateTime(release.created_at) }}</span></template>
      </Column>
      <Column header="操作" style="width: 8rem">
        <template #body="{ data: release }">
          <div class="flex items-center gap-1">
            <Button v-tooltip.bottom="'部署到主机'" severity="secondary" text size="small" class="!h-7 !w-7 !p-0" @click="openDeploy(release)"><Bolt :size="15" /></Button>
            <Button v-tooltip.bottom="'编辑'" severity="secondary" text size="small" class="!h-7 !w-7 !p-0" @click="openEdit(release)"><i class="pi pi-pen-to-square text-[13px]"></i></Button>
            <Button v-tooltip.bottom="'删除'" severity="danger" text size="small" class="!h-7 !w-7 !p-0" @click="remove(release)"><Trash :size="15" /></Button>
          </div>
        </template>
      </Column>
    </DataTable>
  </section>

  <Dialog v-model:visible="uploadVisible" modal header="上传版本" :style="{ width: '42rem', maxWidth: '96vw' }">
    <div class="grid gap-4 sm:grid-cols-2">
      <div class="sm:col-span-2">
        <label class="field-label">项目</label>
        <Select v-model="upload.project_id" :options="projectOptions" option-label="label" option-value="value" placeholder="选择项目" filter fluid />
      </div>
      <div>
        <label class="field-label">版本号</label>
        <InputText v-model="upload.version" placeholder="1.4.0" fluid />
      </div>
      <div>
        <label class="field-label">更新标题</label>
        <InputText v-model="upload.title" placeholder="可选" fluid />
      </div>
      <div>
        <label class="field-label">平台</label>
        <Select v-model="upload.platform" :options="uploadPlatformOptions" option-label="label" option-value="value" fluid />
      </div>
      <div>
        <label class="field-label">架构</label>
        <InputText v-model="upload.arch" placeholder="amd64 / arm64 / universal" fluid />
      </div>
      <div>
        <label class="field-label">通道</label>
        <Select v-model="upload.channel" :options="uploadChannelOptions" option-label="label" option-value="value" fluid />
      </div>
      <div>
        <label class="field-label">灰度比例 %</label>
        <InputNumber v-model="upload.rollout_percent" :min="0" :max="100" show-buttons fluid />
      </div>
      <div class="sm:col-span-2">
        <label class="field-label">更新说明</label>
        <Textarea v-model="upload.changelog" rows="3" auto-resize placeholder="支持 Markdown 文本，客户端可直接展示" fluid />
      </div>
      <div class="sm:col-span-2">
        <label class="field-label">构建产物</label>
        <input ref="fileInput" type="file" class="hidden" @change="pickFile" />
        <div class="flex items-center gap-2">
          <Button severity="secondary" size="small" @click="fileInput?.click()"><CloudUpload :size="15" /> 选择文件</Button>
          <span class="truncate text-xs text-muted-color">{{ upload.file?.name || '支持 .zip / .tar.gz / 单个二进制' }}</span>
        </div>
      </div>
      <label class="flex cursor-pointer items-center justify-between gap-3 rounded-md border border-surface-200 px-3 py-2.5 dark:border-surface-800">
        <span class="text-sm text-color">强制更新</span>
        <ToggleSwitch v-model="upload.force" />
      </label>
      <label class="flex cursor-pointer items-center justify-between gap-3 rounded-md border border-surface-200 px-3 py-2.5 dark:border-surface-800">
        <span class="text-sm text-color">上传后立即部署</span>
        <ToggleSwitch v-model="upload.deploy" />
      </label>
      <div v-if="upload.deploy" class="sm:col-span-2">
        <label class="field-label">目标主机（不选则使用自动部署主机）</label>
        <div v-if="!uploadTargets.length" class="rounded-md border border-dashed border-surface-300 px-3 py-3 text-xs text-muted-color dark:border-surface-700">
          当前项目没有匹配 {{ platformLabel(upload.platform) }} / {{ upload.channel }} 的主机。
        </div>
        <div v-else class="flex flex-wrap gap-3">
          <label v-for="target in uploadTargets" :key="target.id" class="flex cursor-pointer items-center gap-2 rounded-md border border-surface-200 px-3 py-1.5 text-xs dark:border-surface-800">
            <Checkbox v-model="upload.target_ids" :value="target.id" />
            <span class="text-color">{{ target.name }}</span>
            <span class="text-muted-color mono">{{ target.current_version || '未部署' }}</span>
          </label>
        </div>
      </div>
      <Message v-if="uploadError" severity="error" size="small" class="sm:col-span-2">{{ uploadError }}</Message>
    </div>
    <template #footer>
      <Button severity="secondary" text @click="uploadVisible = false">取消</Button>
      <Button :loading="uploading" @click="submitUpload"><CloudUpload :size="15" /> 上传</Button>
    </template>
  </Dialog>

  <Dialog v-model:visible="deployVisible" modal :header="`部署 ${deployReleaseTarget?.version || ''}`" :style="{ width: '36rem', maxWidth: '94vw' }">
    <div class="space-y-3">
      <p class="text-sm text-muted-color">
        选择要部署到的配置。每行对应一台主机上的这个项目，安装路径、重启命令、白名单都取自该配置。
      </p>
      <div v-if="!deployCandidates.length" class="rounded-md border border-dashed border-surface-300 px-3 py-4 text-sm text-muted-color dark:border-surface-700">
        该项目还没有可用的部署配置。
        <RouterLink to="/app/configs" class="ml-1 text-primary hover:underline" @click="deployVisible = false">
          去「部署配置」新建
        </RouterLink>
      </div>
      <div v-else class="space-y-2">
        <label
          v-for="item in deployCandidates"
          :key="item.id"
          class="flex cursor-pointer items-start justify-between gap-3 rounded-md border border-surface-200 px-3 py-2 dark:border-surface-800"
        >
          <span class="flex min-w-0 items-start gap-2">
            <Checkbox v-model="deployConfigIDs" :value="item.id" class="mt-0.5" />
            <span class="min-w-0">
              <span class="block text-sm text-color">{{ item.target_name }}</span>
              <span class="block truncate mono text-[11px] text-muted-color" :title="item.deploy_dir">
                {{ item.deploy_dir || `${item.deploy_root || '/srv'}/${item.project_app_id}` }}
              </span>
              <span class="block text-[11px] text-muted-color">
                当前版本 {{ item.current_version || '未部署' }}
                <span v-if="item.preserve"> · 白名单 {{ item.preserve }}</span>
              </span>
            </span>
          </span>
          <StatusTag :status="item.target_status" kind="target" />
        </label>
      </div>
      <Message v-if="deployError" severity="error" size="small">{{ deployError }}</Message>
    </div>
    <template #footer>
      <Button severity="secondary" text @click="deployVisible = false">取消</Button>
      <Button :loading="deploying" :disabled="!deployCandidates.length" @click="submitDeploy"><Bolt :size="15" /> 开始部署</Button>
    </template>
  </Dialog>

  <Dialog v-model:visible="editVisible" modal :header="`编辑 ${editRelease?.version || ''}`" :style="{ width: '34rem', maxWidth: '94vw' }">
    <div class="flex flex-col gap-4">
      <div>
        <label class="field-label">更新标题</label>
        <InputText v-model="editForm.title" fluid />
      </div>
      <div>
        <label class="field-label">更新说明</label>
        <Textarea v-model="editForm.changelog" rows="4" auto-resize fluid />
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="field-label">灰度比例 %</label>
          <InputNumber v-model="editForm.rollout_percent" :min="0" :max="100" show-buttons fluid />
        </div>
        <div>
          <label class="field-label">最低支持版本号</label>
          <InputNumber v-model="editForm.min_supported_code" :min="0" fluid />
        </div>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <label class="flex cursor-pointer items-center justify-between gap-3 rounded-md border border-surface-200 px-3 py-2.5 dark:border-surface-800">
          <span class="text-sm text-color">启用</span>
          <ToggleSwitch v-model="editForm.enabled" />
        </label>
        <label class="flex cursor-pointer items-center justify-between gap-3 rounded-md border border-surface-200 px-3 py-2.5 dark:border-surface-800">
          <span class="text-sm text-color">强制更新</span>
          <ToggleSwitch v-model="editForm.force" />
        </label>
      </div>
      <Message v-if="editError" severity="error" size="small">{{ editError }}</Message>
    </div>
    <template #footer>
      <Button severity="secondary" text @click="editVisible = false">取消</Button>
      <Button :loading="editSaving" @click="submitEdit">保存</Button>
    </template>
  </Dialog>
</template>
