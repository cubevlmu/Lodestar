<script setup lang="ts">
import Pencil from '@primeicons/vue/pencil'
import Plus from '@primeicons/vue/plus'
import Trash from '@primeicons/vue/trash'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import StatusTag from '../components/StatusTag.vue'
import { api } from '../services/api'
import type { DeployConfig, Project, Target } from '../types'
import { formatDateTime, platformLabel } from '../utils'

const toast = useToast()
const confirm = useConfirm()
const route = useRoute()

const configs = ref<DeployConfig[]>([])
const targets = ref<Target[]>([])
const projects = ref<Project[]>([])
const loading = ref(true)
const error = ref('')
const filterTarget = ref<number | null>(null)

const targetOptions = computed(() => targets.value.map(target => ({ label: `${target.name} · ${platformLabel(target.platform)}`, value: target.id })))
const projectOptions = computed(() => projects.value.map(project => ({ label: `${project.name} · ${project.app_id}`, value: project.id })))

const visibleRows = computed(() =>
  filterTarget.value ? configs.value.filter(item => item.target_id === filterTarget.value) : configs.value,
)

const dialogVisible = ref(false)
const saving = ref(false)
const formError = ref('')
const editing = ref<DeployConfig | null>(null)
const form = reactive({
  target_id: null as number | null,
  project_id: null as number | null,
  deploy_dir: '',
  backup_dir: '',
  stop_command: '',
  start_command: '',
  restart_command: '',
  health_check_url: '',
  preserve: '',
  backup_keep: 5,
})

const selectedTarget = computed(() => targets.value.find(target => target.id === form.target_id) ?? null)
const selectedProject = computed(() => projects.value.find(project => project.id === form.project_id) ?? null)

const suggestedDir = computed(() => {
  if (!selectedTarget.value || !selectedProject.value) return ''
  const existing = configs.value.find(
    item => item.target_id === form.target_id && item.project_id === form.project_id,
  )
  if (existing?.deploy_dir) return existing.deploy_dir
  const root = (selectedTarget.value.deploy_root || '/srv').replace(/\/$/, '')
  return `${root}/${selectedProject.value.app_id}`
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [configResponse, targetResponse, projectResponse] = await Promise.all([
      api.deployConfigs({}),
      api.targets(),
      api.projects(),
    ])
    configs.value = configResponse.items
    targets.value = targetResponse.items
    projects.value = projectResponse.items
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
    target_id: filterTarget.value ?? targets.value[0]?.id ?? null,
    project_id: projects.value[0]?.id ?? null,
    deploy_dir: '',
    backup_dir: '',
    stop_command: '',
    start_command: '',
    restart_command: '',
    health_check_url: '',
    preserve: 'data, config',
    backup_keep: 5,
  })
  form.deploy_dir = suggestedDir.value
  dialogVisible.value = true
}

function openEdit(item: DeployConfig) {
  editing.value = item
  formError.value = ''
  Object.assign(form, {
    target_id: item.target_id,
    project_id: item.project_id,
    deploy_dir: item.deploy_dir,
    backup_dir: item.backup_dir,
    stop_command: item.stop_command,
    start_command: item.start_command,
    restart_command: item.restart_command,
    health_check_url: item.health_check_url,
    preserve: item.preserve,
    backup_keep: item.backup_keep || 5,
  })
  dialogVisible.value = true
}

function syncSuggestedDir() {
  if (!editing.value && !form.deploy_dir) {
    form.deploy_dir = suggestedDir.value
  }
}

async function submit() {
  formError.value = ''
  if (!form.target_id || !form.project_id) {
    formError.value = '请选择主机和项目。'
    return
  }
  saving.value = true
  try {
    await api.updateTargetApp(form.target_id, form.project_id, {
      deploy_dir: form.deploy_dir.trim(),
      backup_dir: form.backup_dir.trim(),
      stop_command: form.stop_command.trim(),
      start_command: form.start_command.trim(),
      restart_command: form.restart_command.trim(),
      health_check_url: form.health_check_url.trim(),
      preserve: form.preserve.trim(),
      backup_keep: form.backup_keep,
    })
    toast.add({ severity: 'success', summary: '部署配置已保存', life: 2200 })
    dialogVisible.value = false
    await load()
  } catch (reason) {
    formError.value = reason instanceof Error ? reason.message : '保存失败'
  } finally {
    saving.value = false
  }
}

function remove(item: DeployConfig) {
  confirm.require({
    header: '删除部署配置',
    message: `确定删除「${item.target_name} / ${item.project_app_id}」的配置吗？只会删掉这条记录，机器上的文件不动。`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    accept: async () => {
      try {
        await api.deleteTargetApp(item.target_id, item.project_id)
        toast.add({ severity: 'success', summary: '配置已删除', life: 2200 })
        await load()
      } catch (reason) {
        toast.add({ severity: 'error', summary: '删除失败', detail: reason instanceof Error ? reason.message : '', life: 4000 })
      }
    },
  })
}

onMounted(async () => {
  const target = Number(route.query.target)
  if (Number.isFinite(target) && target > 0) {
    filterTarget.value = target
  }
  await load()
})
</script>

<template>
  <Teleport to="#page-actions" defer>
    <Select
      v-model="filterTarget"
      :options="targetOptions"
      option-label="label"
      option-value="value"
      placeholder="全部主机"
      show-clear
      class="w-48"
    />
    <Button size="small" @click="openCreate"><Plus :size="16" /> 新增配置</Button>
  </Teleport>

  <Message v-if="error" severity="error" :closable="false" class="mb-4">{{ error }}</Message>

  <section class="surface-panel rounded-lg">
    <DataTable :value="visibleRows" :loading="loading" data-key="id" size="small" class="text-sm">
      <template #empty>
        <div class="table-empty-state">
          还没有部署配置。部署一次会自动创建，也可以点「新增配置」提前为某台主机上的某个项目设定路径与命令。
        </div>
      </template>
      <Column header="主机" class="w-[30%] sm:w-[13rem]">
        <template #body="{ data: item }">
          <div class="min-w-0">
            <div class="truncate text-sm font-semibold text-color">{{ item.target_name }}</div>
            <div class="truncate text-xs text-muted-color">{{ platformLabel(item.target_platform) }}</div>
          </div>
        </template>
      </Column>
      <Column header="应用" class="w-[30%] sm:w-[13rem]">
        <template #body="{ data: item }">
          <div class="min-w-0">
            <div class="truncate text-sm text-color">{{ item.project_name }}</div>
            <div class="truncate text-xs text-muted-color mono">{{ item.project_app_id }}</div>
          </div>
        </template>
      </Column>
      <Column header="部署路径" class="hidden md:table-cell" style="width: 18rem">
        <template #body="{ data: item }">
          <div class="truncate mono text-xs text-color" :title="item.deploy_dir || `${item.deploy_root || '/srv'}/${item.project_app_id}`">
            {{ item.deploy_dir || `${item.deploy_root || '/srv'}/${item.project_app_id}` }}
          </div>
          <span v-if="!item.deploy_dir" class="text-[11px] text-muted-color">默认路径</span>
        </template>
      </Column>
      <Column header="当前版本" class="w-[20%] sm:w-[7rem]">
        <template #body="{ data: item }"><span class="mono text-xs text-color">{{ item.current_version || '未部署' }}</span></template>
      </Column>
      <Column header="白名单（文件/目录）" class="hidden lg:table-cell" style="width: 12rem">
        <template #body="{ data: item }">
          <span class="truncate text-xs text-muted-color">{{ item.preserve || '—' }}</span>
        </template>
      </Column>
      <Column header="状态" class="hidden sm:table-cell" style="width: 6rem">
        <template #body="{ data: item }"><StatusTag :status="item.target_status" kind="target" /></template>
      </Column>
      <Column header="操作" class="w-[18%] sm:w-[6rem]">
        <template #body="{ data: item }">
          <div class="flex items-center gap-1">
            <Button v-tooltip.bottom="'编辑'" severity="secondary" text size="small" class="!h-7 !w-7 !p-0" @click="openEdit(item)"><Pencil :size="15" /></Button>
            <Button v-tooltip.bottom="'删除'" severity="danger" text size="small" class="!h-7 !w-7 !p-0" @click="remove(item)"><Trash :size="15" /></Button>
          </div>
        </template>
      </Column>
    </DataTable>
  </section>

  <Dialog v-model:visible="dialogVisible" modal :header="editing ? '编辑部署配置' : '新增部署配置'" :style="{ width: '42rem', maxWidth: '96vw' }">
    <div class="grid gap-4 sm:grid-cols-2">
      <div>
        <label class="field-label">主机</label>
        <Select
          v-model="form.target_id"
          :options="targetOptions"
          option-label="label"
          option-value="value"
          placeholder="选择主机"
          :disabled="Boolean(editing)"
          fluid
          @update:model-value="syncSuggestedDir"
        />
      </div>
      <div>
        <label class="field-label">项目</label>
        <Select
          v-model="form.project_id"
          :options="projectOptions"
          option-label="label"
          option-value="value"
          placeholder="选择项目"
          filter
          :disabled="Boolean(editing)"
          fluid
          @update:model-value="syncSuggestedDir"
        />
      </div>
      <div class="sm:col-span-2">
        <label class="field-label">部署路径</label>
        <InputText v-model="form.deploy_dir" :placeholder="suggestedDir || '/srv/demo-app'" fluid />
        <p class="mt-1 text-xs text-muted-color">
          这台机器上的绝对路径，任意目录都行。留空则用
          <span class="mono">{{ suggestedDir || '<部署根目录>/<app_id>' }}</span>。
        </p>
      </div>
      <div>
        <label class="field-label">备份目录（可选）</label>
        <InputText v-model="form.backup_dir" placeholder="留空则用 &lt;部署目录&gt;.backups" fluid />
      </div>
      <div>
        <label class="field-label">保留备份数量</label>
        <InputNumber v-model="form.backup_keep" :min="1" :max="50" show-buttons fluid />
      </div>
      <div class="sm:col-span-2">
        <label class="field-label">白名单（文件或目录）</label>
        <InputText v-model="form.preserve" placeholder="appsettings.json, data, uploads" fluid />
        <p class="mt-1 text-xs text-muted-color">
          相对部署目录，文件或目录都可以，多个用逗号分隔，例如
          <span class="mono">appsettings.json</span>、<span class="mono">data</span>、<span class="mono">logs/app.log</span>。
          这些内容更新时不覆盖、备份不包含、回滚也不覆盖；首次安装时仍会用包里的默认值。
        </p>
      </div>
      <div class="sm:col-span-2">
        <p class="text-xs leading-5 text-muted-color">下面几条命令全部可选，留空就只替换文件、不动进程。</p>
      </div>
      <div class="sm:col-span-2">
        <label class="field-label">停止命令（可选）</label>
        <InputText v-model="form.stop_command" placeholder="systemctl stop demo" fluid />
      </div>
      <div class="sm:col-span-2">
        <label class="field-label">启动命令（可选，首次安装时用）</label>
        <InputText v-model="form.start_command" placeholder="systemctl start demo" fluid />
      </div>
      <div class="sm:col-span-2">
        <label class="field-label">重启命令（可选，换版时用）</label>
        <InputText v-model="form.restart_command" placeholder="systemctl restart demo" fluid />
      </div>
      <div class="sm:col-span-2">
        <label class="field-label">健康检查 URL（可选）</label>
        <InputText v-model="form.health_check_url" placeholder="http://127.0.0.1:9000/healthz" fluid />
        <p class="mt-1 text-xs text-muted-color">配了才轮询；连续失败会触发自动回滚。</p>
      </div>
      <Message v-if="formError" severity="error" size="small" class="sm:col-span-2">{{ formError }}</Message>
    </div>
    <template #footer>
      <Button severity="secondary" text @click="dialogVisible = false">取消</Button>
      <Button :loading="saving" @click="submit">保存</Button>
    </template>
  </Dialog>
</template>
