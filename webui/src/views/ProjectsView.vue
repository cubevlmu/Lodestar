<script setup lang="ts">
import Key from '@primeicons/vue/key'
import Plus from '@primeicons/vue/plus'
import Trash from '@primeicons/vue/trash'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import Drawer from 'primevue/drawer'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import { computed, onMounted, reactive, ref } from 'vue'
import CopyButton from '../components/CopyButton.vue'
import Tag from 'primevue/tag'
import { api } from '../services/api'
import type { Project } from '../types'
import { formatDateTime } from '../utils'

const toast = useToast()
const confirm = useConfirm()

const projects = ref<Project[]>([])
const loading = ref(true)
const error = ref('')

const formVisible = ref(false)
const saving = ref(false)
const formError = ref('')
const editing = ref<Project | null>(null)
const form = reactive({ app_id: '', name: '', description: '', repo_url: '', enabled: true })

const detailVisible = ref(false)
const detail = ref<Project | null>(null)

const isEditing = computed(() => Boolean(editing.value))
const deployEndpoint = computed(() => `${window.location.origin}/api/v1/deploy`)
const curlSnippet = computed(() => {
  const token = detail.value?.deploy_token || '<deploy_token>'
  return `curl -X POST ${deployEndpoint.value} \\
  -H "X-Deploy-Token: ${token}" \\
  -F version=1.4.0 \\
  -F platform=linux \\
  -F arch=amd64 \\
  -F channel=stable \\
  -F changelog="修复若干问题" \\
  -F package=@dist/app-linux-amd64.zip \\
  -F wait_seconds=180`
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const response = await api.projects()
    projects.value = response.items
    if (detail.value) {
      const fresh = response.items.find(item => item.id === detail.value?.id)
      if (fresh) detail.value = fresh
    }
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '加载失败'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  formError.value = ''
  Object.assign(form, { app_id: '', name: '', description: '', repo_url: '', enabled: true })
  formVisible.value = true
}

function openEdit(project: Project) {
  editing.value = project
  formError.value = ''
  Object.assign(form, {
    app_id: project.app_id,
    name: project.name,
    description: project.description,
    repo_url: project.repo_url,
    enabled: project.enabled,
  })
  formVisible.value = true
}

async function submit() {
  formError.value = ''
  if (!isEditing.value && !/^[a-z0-9][a-z0-9._-]{1,127}$/.test(form.app_id.trim())) {
    formError.value = 'app_id 只能包含小写字母、数字、点、下划线和短横线，长度 2-128。'
    return
  }
  if (!form.name.trim()) {
    formError.value = '请输入项目名称。'
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      await api.updateProject(editing.value.id, {
        name: form.name,
        description: form.description,
        repo_url: form.repo_url,
        enabled: form.enabled,
      })
      toast.add({ severity: 'success', summary: '项目已更新', life: 2200 })
    } else {
      const response = await api.createProject({
        app_id: form.app_id.trim(),
        name: form.name,
        description: form.description,
        repo_url: form.repo_url,
      })
      toast.add({ severity: 'success', summary: '项目已创建', detail: '请保存部署令牌，它只在此处完整显示。', life: 3600 })
      detail.value = response.project
      detailVisible.value = true
    }
    formVisible.value = false
    await load()
  } catch (reason) {
    formError.value = reason instanceof Error ? reason.message : '保存失败'
  } finally {
    saving.value = false
  }
}

function openDetail(project: Project) {
  detail.value = project
  detailVisible.value = true
}

async function rotate(kind: 'client_secret' | 'deploy_token' | 'ed25519') {
  if (!detail.value) return
  const labels: Record<string, string> = {
    client_secret: '客户端签名密钥',
    deploy_token: 'CI 部署令牌',
    ed25519: 'Ed25519 密钥对',
  }
  confirm.require({
    header: '轮换凭据',
    message: `确定轮换「${labels[kind]}」吗？旧的凭据会立即失效。`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '轮换',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    accept: async () => {
      try {
        const response = await api.rotateProject(detail.value!.id, kind)
        detail.value = { ...detail.value!, ...response.project }
        toast.add({ severity: 'success', summary: '凭据已轮换', life: 2400 })
        await load()
      } catch (reason) {
        toast.add({ severity: 'error', summary: '轮换失败', detail: reason instanceof Error ? reason.message : '', life: 4000 })
      }
    },
  })
}

function remove(project: Project) {
  confirm.require({
    header: '删除项目',
    message: `删除「${project.name}」会同时移除它的版本、目标主机与部署记录，且不可恢复。`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: '删除',
    rejectLabel: '取消',
    acceptProps: { severity: 'danger' },
    accept: async () => {
      try {
        await api.deleteProject(project.id)
        if (detail.value?.id === project.id) detailVisible.value = false
        toast.add({ severity: 'success', summary: '项目已删除', life: 2200 })
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
    <Button size="small" @click="openCreate"><Plus :size="16" /> 新建项目</Button>
  </Teleport>

  <Message v-if="error" severity="error" :closable="false" class="mb-4">{{ error }}</Message>

  <section class="surface-panel rounded-lg">
    <DataTable :value="projects" :loading="loading" data-key="id" size="small" row-hover class="text-sm" @row-click="event => openDetail(event.data)">
      <template #empty><div class="table-empty-state">还没有项目。创建一个项目来管理应用版本与部署凭据。</div></template>
      <Column header="项目">
        <template #body="{ data: project }">
          <div class="min-w-0">
            <div class="truncate text-sm font-semibold text-color">{{ project.name }}</div>
            <div class="truncate text-xs text-muted-color mono">{{ project.app_id }}</div>
          </div>
        </template>
      </Column>
      <Column header="版本" class="hidden sm:table-cell" style="width: 7rem">
        <template #body="{ data: project }">
          <span class="mono text-xs text-color">{{ project.release_count ?? 0 }}</span>
        </template>
      </Column>
      <Column header="主机" class="hidden sm:table-cell" style="width: 8rem">
        <template #body="{ data: project }">
          <span class="text-xs text-color">{{ project.online_targets ?? 0 }}/{{ project.target_count ?? 0 }} 在线</span>
        </template>
      </Column>
      <Column header="最新版本" class="hidden md:table-cell" style="width: 9rem">
        <template #body="{ data: project }">
          <span class="mono text-xs text-color">{{ project.latest_version || '—' }}</span>
        </template>
      </Column>
      <Column header="状态" style="width: 6rem">
        <template #body="{ data: project }">
          <Tag :value="project.enabled ? '启用' : '停用'" :severity="project.enabled ? 'success' : 'secondary'" class="!px-2 !py-0.5 !text-xs" />
        </template>
      </Column>
      <Column header="操作" style="width: 8.5rem">
        <template #body="{ data: project }">
          <div class="flex items-center gap-1" @click.stop>
            <Button v-tooltip.bottom="'凭据'" severity="secondary" text size="small" class="!h-7 !w-7 !p-0" @click="openDetail(project)"><Key :size="15" /></Button>
            <Button v-tooltip.bottom="'编辑'" severity="secondary" text size="small" class="!h-7 !w-7 !p-0" @click="openEdit(project)"><i class="pi pi-pen-to-square text-[13px]"></i></Button>
            <Button v-tooltip.bottom="'删除'" severity="danger" text size="small" class="!h-7 !w-7 !p-0" @click="remove(project)"><Trash :size="15" /></Button>
          </div>
        </template>
      </Column>
    </DataTable>
  </section>

  <Dialog v-model:visible="formVisible" modal :header="isEditing ? '编辑项目' : '新建项目'" :style="{ width: '32rem', maxWidth: '94vw' }">
    <div class="flex flex-col gap-4">
      <div>
        <label class="field-label" for="project-app-id">应用标识 app_id</label>
        <InputText id="project-app-id" v-model="form.app_id" :disabled="isEditing" placeholder="com.example.app" fluid />
        <p class="mt-1.5 text-xs text-muted-color">客户端请求头 X-App-Id 使用的值，创建后不可修改。</p>
      </div>
      <div>
        <label class="field-label" for="project-name">项目名称</label>
        <InputText id="project-name" v-model="form.name" placeholder="例如：订单服务" fluid />
      </div>
      <div>
        <label class="field-label" for="project-desc">描述</label>
        <InputText id="project-desc" v-model="form.description" placeholder="可选" fluid />
      </div>
      <div>
        <label class="field-label" for="project-repo">仓库地址</label>
        <InputText id="project-repo" v-model="form.repo_url" placeholder="https://github.com/owner/repo" fluid />
      </div>
      <label v-if="isEditing" class="flex cursor-pointer items-center justify-between gap-3 rounded-md border border-surface-200 px-3 py-2.5 dark:border-surface-800">
        <span class="text-sm text-color">启用项目</span>
        <ToggleSwitch v-model="form.enabled" />
      </label>
      <Message v-if="formError" severity="error" size="small">{{ formError }}</Message>
    </div>
    <template #footer>
      <Button severity="secondary" text @click="formVisible = false">取消</Button>
      <Button :loading="saving" @click="submit">{{ isEditing ? '保存' : '创建' }}</Button>
    </template>
  </Dialog>

  <Drawer v-model:visible="detailVisible" position="right" :style="{ width: '36rem', maxWidth: '96vw' }">
    <template #header>
      <div v-if="detail" class="min-w-0">
        <div class="truncate text-base font-semibold text-color">{{ detail.name }}</div>
        <div class="truncate text-xs text-muted-color mono">{{ detail.app_id }}</div>
      </div>
    </template>

    <div v-if="detail" class="space-y-5">
      <div class="surface-panel rounded-lg p-3">
        <div class="detail-row">
          <span>创建时间</span>
          <strong>{{ formatDateTime(detail.created_at) }}</strong>
        </div>
        <div class="detail-row">
          <span>仓库</span>
          <strong class="mono">{{ detail.repo_url || '—' }}</strong>
        </div>
        <div class="detail-row">
          <span>描述</span>
          <strong>{{ detail.description || '—' }}</strong>
        </div>
      </div>

      <section>
        <h3 class="text-sm font-semibold text-color">部署凭据</h3>
        <p class="mt-1 text-xs text-muted-color">CI 使用部署令牌调用发布接口；客户端更新校验使用签名密钥。</p>
        <div class="mt-3 space-y-2">
          <div class="credential-row">
            <div class="min-w-0">
              <div class="credential-label">CI 部署令牌</div>
              <div class="credential-value">{{ detail.deploy_token || '—' }}</div>
            </div>
            <div class="flex shrink-0 items-center gap-1">
              <CopyButton :value="detail.deploy_token || ''" />
              <Button v-tooltip.bottom="'轮换'" severity="secondary" text size="small" class="!h-7 !w-7 !p-0" @click="rotate('deploy_token')"><i class="pi pi-refresh text-[13px]"></i></Button>
            </div>
          </div>
          <div class="credential-row">
            <div class="min-w-0">
              <div class="credential-label">客户端签名密钥 client_secret</div>
              <div class="credential-value">{{ detail.client_secret || '—' }}</div>
            </div>
            <div class="flex shrink-0 items-center gap-1">
              <CopyButton :value="detail.client_secret || ''" />
              <Button v-tooltip.bottom="'轮换'" severity="secondary" text size="small" class="!h-7 !w-7 !p-0" @click="rotate('client_secret')"><i class="pi pi-refresh text-[13px]"></i></Button>
            </div>
          </div>
          <div class="credential-row">
            <div class="min-w-0">
              <div class="credential-label">Ed25519 公钥</div>
              <div class="credential-value">{{ detail.ed25519_public_key }}</div>
            </div>
            <div class="flex shrink-0 items-center gap-1">
              <CopyButton :value="detail.ed25519_public_key" />
              <Button v-tooltip.bottom="'轮换密钥对'" severity="secondary" text size="small" class="!h-7 !w-7 !p-0" @click="rotate('ed25519')"><i class="pi pi-refresh text-[13px]"></i></Button>
            </div>
          </div>
        </div>
      </section>

      <section>
        <h3 class="text-sm font-semibold text-color">从 CI 发布</h3>
        <p class="mt-1 text-xs text-muted-color">构建完成后上传产物，服务端会登记版本并给匹配的目标主机排队部署。</p>
        <div class="code-block mt-3">{{ curlSnippet }}</div>
        <div class="mt-2 flex items-center gap-2">
          <CopyButton :value="curlSnippet" label="复制示例" />
          <span class="text-xs text-muted-color">接口：<span class="mono">POST /api/v1/deploy</span></span>
        </div>
      </section>

      <div class="flex items-center justify-between gap-2 border-t border-surface-200 pt-4 dark:border-surface-800">
        <Button severity="danger" text size="small" @click="remove(detail)"><Trash :size="15" /> 删除项目</Button>
        <Button severity="secondary" size="small" @click="openEdit(detail)"><i class="pi pi-pen-to-square"></i> 编辑</Button>
      </div>
    </div>
  </Drawer>
</template>

<style scoped>
.detail-row {
  display: grid;
  grid-template-columns: minmax(6rem, 0.6fr) minmax(0, 1.4fr);
  gap: 1rem;
  border-bottom: 1px solid var(--p-content-border-color);
  padding: 0.55rem 0;
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
