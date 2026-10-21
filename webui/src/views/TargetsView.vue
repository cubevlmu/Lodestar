<script setup lang="ts">
import Bolt from '@primeicons/vue/bolt'
import Key from '@primeicons/vue/key'
import Pencil from '@primeicons/vue/pencil'
import Plus from '@primeicons/vue/plus'
import Server from '@primeicons/vue/server'
import Trash from '@primeicons/vue/trash'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Menu from 'primevue/menu'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import CopyButton from '../components/CopyButton.vue'
import StatusTag from '../components/StatusTag.vue'
import { api } from '../services/api'
import type { Target } from '../types'
import { formatDateTime, formatRelative, platformLabel } from '../utils'

const toast = useToast()
const router = useRouter()
const confirm = useConfirm()

const targets = ref<Target[]>([])
const loading = ref(true)
const error = ref('')

const platformOptions = [
  { label: 'Linux', value: 'linux' },
  { label: 'Windows', value: 'windows' },
  { label: 'macOS', value: 'darwin' },
]
const channelOptions = [
  { label: 'stable', value: 'stable' },
  { label: 'beta', value: 'beta' },
  { label: 'dev', value: 'dev' },
]

const formVisible = ref(false)
const saving = ref(false)
const formError = ref('')
const editing = ref<Target | null>(null)
const form = reactive({
  name: '',
  platform: 'linux',
  arch: 'amd64',
  channel: 'stable',
  deploy_root: '/srv',
  backup_dir: '',
  auto_deploy: true,
  notes: '',
})

const tokenVisible = ref(false)
const tokenTarget = ref<Target | null>(null)

const deployVisible = ref(false)
const deployTarget = ref<Target | null>(null)
const deployReleaseID = ref<number | null>(null)
const deployLoading = ref(false)
const deployError = ref('')
const releaseOptions = ref<{ label: string; value: number }[]>([])
// Deploying always picks one of this host's saved configs; the path and
// commands come from there instead of being typed again.
const deployAppID = ref<number | null>(null)
const deployApps = computed(() => deployTarget.value?.apps ?? [])
const deployApp = computed(() => deployApps.value.find(app => app.id === deployAppID.value) ?? null)
const deployAppOptions = computed(() =>
  deployApps.value.map(app => ({
    label: `${app.project_app_id} · ${app.deploy_dir || '默认路径'}`,
    value: app.id,
  })),
)

const agentSnippet = computed(() => {
  const target = tokenTarget.value
  if (!target) return ''
  const root = target.deploy_root || '/srv'
  return `# 1. 在服务器上准备配置（与 lodestar 可执行文件同目录）
#    data/config/system.yaml
agent:
  server_url: ${window.location.origin}
  target_token: "${target.token || '<target_token>'}"
  name: "${target.name}"
  # 默认安装根目录；每个应用默认装到 <root>/<app_id>。
  # 部署路径、重启命令、白名单都在 WebUI 的「部署配置」里按项目设置，
  # 这里不再配置任何命令。
  deploy_root: ${root}
  work_dir: /var/lib/lodestar

# 2. 以服务方式启动 Agent
./lodestar agent
# 建议交给 systemd 托管：
#   ExecStart=/opt/lodestar/lodestar agent
#   Restart=always`
})

// Phones get a single overflow button instead of four inline icon buttons,
// which is what makes the host table fit without horizontal scrolling.
const rowMenu = ref<InstanceType<typeof Menu> | null>(null)
const rowMenuTarget = ref<Target | null>(null)

const rowMenuItems = computed(() => {
  const target = rowMenuTarget.value
  if (!target) return []
  return [
    { label: '部署版本', icon: Bolt, command: () => openDeploy(target) },
    { label: '接入配置', icon: Key, command: () => showToken(target) },
    { label: '编辑主机', icon: Pencil, command: () => openEdit(target) },
    { separator: true },
    { label: '删除主机', icon: Trash, command: () => remove(target) },
  ]
})

function openRowMenu(event: Event, target: Target) {
  rowMenuTarget.value = target
  rowMenu.value?.toggle(event)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const response = await api.targets()
    targets.value = response.items
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '加载失败'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  formError.value = ''
  Object.assign(form, {
    name: '',
    platform: 'linux',
    arch: 'amd64',
    channel: 'stable',
    deploy_root: '/srv',
    backup_dir: '',
    auto_deploy: true,
    notes: '',
  })
  formVisible.value = true
}

function openEdit(target: Target) {
  editing.value = target
  formError.value = ''
  Object.assign(form, {
    name: target.name,
    platform: target.platform,
    arch: target.arch,
    channel: target.channel,
    deploy_root: target.deploy_root,
    backup_dir: target.backup_dir,
    auto_deploy: target.auto_deploy,
    notes: target.notes,
  })
  formVisible.value = true
}

async function submit() {
  formError.value = ''
  if (!form.name.trim()) {
    formError.value = '请输入主机名称。'
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      const response = await api.updateTarget(editing.value.id, { ...form } as Partial<Target>)
      toast.add({ severity: 'success', summary: '目标主机已更新', life: 2200 })
      tokenTarget.value = { ...editing.value, ...response.target }
      formVisible.value = false
      await load()
    } else {
      const response = await api.createTarget({ ...form } as Partial<Target>)
      formVisible.value = false
      tokenTarget.value = response.target
      tokenVisible.value = true
      toast.add({ severity: 'success', summary: '目标主机已创建', detail: '复制令牌并部署 Agent。', life: 3200 })
      await load()
    }
  } catch (reason) {
    formError.value = reason instanceof Error ? reason.message : '保存失败'
  } finally {
    saving.value = false
  }
}

function showToken(target: Target) {
  tokenTarget.value = target
  tokenVisible.value = true
}

async function rotateToken() {
  if (!tokenTarget.value) return
  try {
    const response = await api.rotateTarget(tokenTarget.value.id)
    tokenTarget.value = response.target
    toast.add({ severity: 'success', summary: '令牌已轮换', detail: '记得更新服务器上的 agent 配置。', life: 3600 })
    await load()
  } catch (reason) {
    toast.add({ severity: 'error', summary: '轮换失败', detail: reason instanceof Error ? reason.message : '', life: 4000 })
  }
}

async function loadDeployReleases() {
  const target = deployTarget.value
  const app = deployApp.value
  if (!target || !app) {
    releaseOptions.value = []
    return
  }
  deployLoading.value = true
  try {
    const response = await api.releases({ project_id: app.project_id, platform: target.platform, size: 50 })
    releaseOptions.value = response.items
      .filter(release => release.channel === target.channel)
      .map(release => ({ label: `${release.version} · ${release.channel}${release.enabled ? '' : '（已停用）'}`, value: release.id }))
    deployReleaseID.value = releaseOptions.value[0]?.value ?? null
  } catch {
    releaseOptions.value = []
  } finally {
    deployLoading.value = false
  }
}

function onDeployAppChange() {
  deployReleaseID.value = null
  void loadDeployReleases()
}

async function openDeploy(target: Target) {
  deployTarget.value = target
  deployReleaseID.value = null
  deployError.value = ''
  deployVisible.value = true
  deployAppID.value = target.apps[0]?.id ?? null
  await loadDeployReleases()
}

async function submitDeploy() {
  if (!deployTarget.value || !deployReleaseID.value) {
    deployError.value = '请选择要部署的版本。'
    return
  }
  deployLoading.value = true
  deployError.value = ''
  try {
    await api.deployTarget(deployTarget.value.id, { release_id: deployReleaseID.value })
    toast.add({ severity: 'success', summary: '部署任务已排队', detail: 'Agent 会在下一次轮询时执行。', life: 3200 })
    deployVisible.value = false
    await load()
  } catch (reason) {
    deployError.value = reason instanceof Error ? reason.message : '部署失败'
  } finally {
    deployLoading.value = false
  }
}

function remove(target: Target) {
  confirm.require({
    header: '删除目标主机',
    message: `确定删除「${target.name}」吗？它的部署记录也会一并删除，服务器上的 Agent 将无法再上报。`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    accept: async () => {
      try {
        await api.deleteTarget(target.id)
        toast.add({ severity: 'success', summary: '目标主机已删除', life: 2200 })
        await load()
      } catch (reason) {
        toast.add({ severity: 'error', summary: '删除失败', detail: reason instanceof Error ? reason.message : '', life: 4000 })
      }
    },
  })
}

onMounted(load)
</script>

<template>
  <Teleport to="#page-actions" defer>
    <Button size="small" @click="openCreate"><Plus :size="16" /> 登记主机</Button>
  </Teleport>

  <Message v-if="error" severity="error" :closable="false" class="mb-4">{{ error }}</Message>

  <section class="surface-panel rounded-lg">
    <DataTable :value="targets" :loading="loading" data-key="id" size="small" class="text-sm">
      <template #empty>
        <div class="table-empty-state">
          还没有目标主机。点击「登记主机」创建令牌，然后在生产服务器上启动 <span class="mono">lodestar agent</span>。
        </div>
      </template>
      <Column header="主机" class="w-[34%] sm:w-[15rem]">
        <template #body="{ data: target }">
          <div class="flex items-center gap-2">
            <span class="hidden size-7 shrink-0 items-center justify-center rounded-md bg-surface-100 text-muted-color dark:bg-surface-800 sm:flex"><Server :size="15" /></span>
            <div class="min-w-0">
              <div class="truncate text-sm font-semibold text-color">{{ target.name }}</div>
              <div class="truncate text-xs text-muted-color">
                {{ target.hostname || '等待首次心跳' }}
                <span v-if="target.os" class="mono"> · {{ target.os }}</span>
              </div>
            </div>
          </div>
        </template>
      </Column>
      <Column header="应用" class="hidden sm:table-cell" style="width: 14rem">
        <template #body="{ data: target }">
          <div class="flex flex-wrap gap-1">
            <button
              v-for="app in target.apps"
              :key="app.id"
              type="button"
              class="rounded bg-surface-100 px-1.5 py-0.5 text-[11px] text-color transition hover:bg-primary-100 hover:text-primary-900 dark:bg-surface-800 dark:hover:bg-primary-900/40 dark:hover:text-primary-100"
              :title="`配置 ${app.project_app_id}`"
              @click.stop="router.push({ path: '/app/configs', query: { target: String(target.id) } })"
            >
              {{ app.project_app_id }}
              <span class="mono text-muted-color">{{ app.current_version || '未部署' }}</span>
            </button>
            <span v-if="!target.apps.length" class="text-xs text-muted-color">尚未部署应用</span>
          </div>
        </template>
      </Column>
      <Column header="平台" class="hidden md:table-cell" style="width: 8.5rem">
        <template #body="{ data: target }">
          <span class="text-xs text-color">{{ platformLabel(target.platform) }}</span>
          <span class="ml-1 text-[11px] text-muted-color mono">{{ target.arch }}</span>
        </template>
      </Column>
      <Column header="部署根目录" class="w-[26%] sm:w-[9rem]">
        <template #body="{ data: target }"><span class="mono truncate text-xs text-muted-color">{{ target.deploy_root || '未设置' }}</span></template>
      </Column>
      <Column header="状态" class="w-[22%] sm:w-[6rem]">
        <template #body="{ data: target }"><StatusTag :status="target.status" kind="target" /></template>
      </Column>
      <Column header="最后心跳" class="hidden sm:table-cell" style="width: 8rem">
        <template #body="{ data: target }"><span class="text-xs text-muted-color">{{ formatRelative(target.last_seen_at) }}</span></template>
      </Column>
      <Column header="最近部署" class="hidden xl:table-cell" style="width: 8rem">
        <template #body="{ data: target }"><span class="text-xs text-muted-color">{{ formatDateTime(target.last_deploy_at) }}</span></template>
      </Column>
      <Column header="操作" class="w-[18%] sm:w-[9.5rem]">
        <template #body="{ data: target }">
          <div class="hidden items-center gap-1 sm:flex">
            <Button v-tooltip.bottom="'部署版本'" severity="secondary" text size="small" class="!h-7 !w-7 !p-0" @click="openDeploy(target)"><Bolt :size="15" /></Button>
            <Button v-tooltip.bottom="'接入配置'" severity="secondary" text size="small" class="!h-7 !w-7 !p-0" @click="showToken(target)"><Key :size="15" /></Button>
            <Button v-tooltip.bottom="'编辑'" severity="secondary" text size="small" class="!h-7 !w-7 !p-0" @click="openEdit(target)"><i class="pi pi-pen-to-square text-[13px]"></i></Button>
            <Button v-tooltip.bottom="'删除'" severity="danger" text size="small" class="!h-7 !w-7 !p-0" @click="remove(target)"><Trash :size="15" /></Button>
          </div>
          <Button
            severity="secondary"
            text
            size="small"
            class="!h-7 !w-7 !p-0 sm:hidden"
            aria-label="主机操作"
            @click.stop="openRowMenu($event, target)"
          >
            <i class="pi pi-ellipsis-h text-[15px]"></i>
          </Button>
        </template>
      </Column>
    </DataTable>
  </section>

  <Menu ref="rowMenu" :model="rowMenuItems" popup />

  <Dialog v-model:visible="formVisible" modal :header="editing ? '编辑目标主机' : '登记目标主机'" :style="{ width: '42rem', maxWidth: '96vw' }">
    <div class="grid gap-4 sm:grid-cols-2">
      <div>
        <label class="field-label">主机名称</label>
        <InputText v-model="form.name" placeholder="prod-api-1" fluid />
      </div>
      <div>
        <label class="field-label">部署根目录</label>
        <InputText v-model="form.deploy_root" placeholder="/srv" fluid />
        <p class="mt-1 text-xs text-muted-color">这台机器允许安装应用的位置。应用默认装到 <span class="mono">&lt;根目录&gt;/&lt;app_id&gt;</span>，部署时可逐个改。</p>
      </div>
      <div>
        <label class="field-label">平台</label>
        <Select v-model="form.platform" :options="platformOptions" option-label="label" option-value="value" fluid />
      </div>
      <div>
        <label class="field-label">架构</label>
        <InputText v-model="form.arch" placeholder="amd64 / arm64 / universal" fluid />
      </div>
      <div>
        <label class="field-label">通道</label>
        <Select v-model="form.channel" :options="channelOptions" option-label="label" option-value="value" fluid />
      </div>
      <div>
        <label class="field-label">备份目录（可选）</label>
        <InputText v-model="form.backup_dir" placeholder="留空则用 &lt;部署目录&gt;.backups" fluid />
      </div>
      <p class="text-xs leading-5 text-muted-color sm:col-span-2">
        重启命令、健康检查、白名单都是**按项目**配置的：登记完主机后，在表格的「应用」列点开对应项目即可设置。
      </p>
      <label class="flex cursor-pointer items-center justify-between gap-3 rounded-md border border-surface-200 px-3 py-2.5 dark:border-surface-800">
        <span class="text-sm text-color">自动部署</span>
        <ToggleSwitch v-model="form.auto_deploy" />
      </label>
      <div class="sm:col-span-2">
        <label class="field-label">备注</label>
        <Textarea v-model="form.notes" rows="2" auto-resize placeholder="可选" fluid />
      </div>
      <Message v-if="formError" severity="error" size="small" class="sm:col-span-2">{{ formError }}</Message>
    </div>
    <template #footer>
      <Button severity="secondary" text @click="formVisible = false">取消</Button>
      <Button :loading="saving" @click="submit">保存</Button>
    </template>
  </Dialog>

  <Dialog v-model:visible="tokenVisible" modal header="Agent 接入配置" :style="{ width: '44rem', maxWidth: '96vw' }">
    <div v-if="tokenTarget" class="space-y-4">
      <div class="credential-row">
        <div class="min-w-0">
          <div class="credential-label">目标令牌 target_token</div>
          <div class="credential-value">{{ tokenTarget.token || '（已隐藏，可轮换生成新令牌）' }}</div>
        </div>
        <div class="flex shrink-0 items-center gap-1">
          <CopyButton :value="tokenTarget.token || ''" />
          <Button severity="secondary" size="small" @click="rotateToken"><i class="pi pi-refresh"></i> 轮换</Button>
        </div>
      </div>
      <div>
        <div class="mb-2 flex items-center justify-between">
          <span class="text-sm font-semibold text-color">服务器上的配置</span>
          <CopyButton :value="agentSnippet" label="复制配置" />
        </div>
        <div class="code-block">{{ agentSnippet }}</div>
      </div>
      <p class="text-xs leading-5 text-muted-color">
        这台机器只配置一个默认的 <span class="mono">deploy_root</span>，实际部署路径在每次部署时指定，
        所以同一台 Agent 可以服务多个项目，装到机器上的任意目录都行。
      </p>
    </div>
  </Dialog>

  <Dialog v-model:visible="deployVisible" modal :header="`部署到 ${deployTarget?.name || ''}`" :style="{ width: '34rem', maxWidth: '94vw' }">
    <div class="space-y-4">
      <div v-if="!deployApps.length" class="rounded-md border border-dashed border-surface-300 px-3 py-4 text-sm text-muted-color dark:border-surface-700">
        这台主机还没有部署配置。
        <RouterLink :to="{ path: '/app/configs', query: { target: String(deployTarget?.id || '') } }" class="ml-1 text-primary hover:underline" @click="deployVisible = false">
          去「部署配置」新建
        </RouterLink>
      </div>
      <template v-else>
        <div>
          <label class="field-label">部署配置</label>
          <Select
            v-model="deployAppID"
            :options="deployAppOptions"
            option-label="label"
            option-value="value"
            placeholder="选择应用"
            fluid
            @update:model-value="onDeployAppChange"
          />
          <p class="mt-1 text-xs text-muted-color">
            安装路径、重启命令与白名单都取自该配置，可在「部署配置」里修改。
          </p>
        </div>
        <div>
          <label class="field-label">选择版本</label>
          <Select v-model="deployReleaseID" :options="releaseOptions" option-label="label" option-value="value" placeholder="选择版本" :loading="deployLoading" fluid />
        </div>
      </template>
      <p v-if="!deployLoading && !releaseOptions.length" class="text-xs text-muted-color">
        该项目在 {{ deployTarget ? platformLabel(deployTarget.platform) : '' }} / {{ deployTarget?.channel }} 下还没有版本包。
      </p>
      <Message v-if="deployError" severity="error" size="small">{{ deployError }}</Message>
    </div>
    <template #footer>
      <Button severity="secondary" text @click="deployVisible = false">取消</Button>
      <Button :loading="deployLoading" :disabled="!deployApps.length || !releaseOptions.length" @click="submitDeploy"><Bolt :size="15" /> 开始部署</Button>
    </template>
  </Dialog>
</template>
