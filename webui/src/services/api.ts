import type {
  AuditLog,
  BlockedVersion,
  DeployJob,
  DeployLog,
  Overview,
  Paged,
  Project,
  Release,
  DeployConfig,
  ServerSettings,
  Target,
  TargetApp,
  User,
  VersionPolicy,
} from '../types'

const base = '/api/v1'
const tokenKey = 'lodestar-token'

export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export function getToken(): string | null {
  try {
    return localStorage.getItem(tokenKey)
  } catch {
    return null
  }
}

export function setToken(token: string) {
  localStorage.setItem(tokenKey, token)
}

export function clearToken() {
  localStorage.removeItem(tokenKey)
}

function authHeaders(): Record<string, string> {
  const token = getToken()
  return token ? { Authorization: `Bearer ${token}` } : {}
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = { ...authHeaders(), ...(init.headers as Record<string, string> | undefined) }
  if (init.body && !(init.body instanceof FormData)) headers['Content-Type'] = 'application/json'

  const response = await fetch(`${base}${path}`, { ...init, headers })
  if (response.status === 204) return undefined as T

  const text = await response.text()
  let payload: any = null
  if (text) {
    try {
      payload = JSON.parse(text)
    } catch {
      payload = { error: text }
    }
  }

  if (!response.ok) {
    if (response.status === 401 && getToken() && !path.startsWith('/auth/login')) {
      clearToken()
      if (!window.location.pathname.startsWith('/login')) window.location.assign('/login')
    }
    throw new ApiError(response.status, payload?.error || `请求失败 (HTTP ${response.status})`)
  }
  return payload as T
}

function query(params: Record<string, string | number | undefined | null>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '') continue
    search.set(key, String(value))
  }
  const encoded = search.toString()
  return encoded ? `?${encoded}` : ''
}

export const api = {
  // auth
  login: (username: string, password: string) =>
    request<{ token: string; expires_at: string; user: User }>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  logout: () => request<{ status: string }>('/auth/logout', { method: 'POST' }),
  me: () => request<{ user: User; server_now: string; uptime: number }>('/auth/me'),
  changePassword: (current_password: string, new_password: string) =>
    request<{ status: string }>('/auth/password', { method: 'POST', body: JSON.stringify({ current_password, new_password }) }),

  // dashboard
  overview: () => request<Overview>('/overview'),

  // projects
  projects: () => request<{ items: Project[] }>('/projects'),
  createProject: (payload: Partial<Project>) =>
    request<{ project: Project }>('/projects', { method: 'POST', body: JSON.stringify(payload) }),
  updateProject: (id: number, payload: Partial<Project>) =>
    request<{ project: Project }>(`/projects/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  deleteProject: (id: number) => request<{ status: string }>(`/projects/${id}`, { method: 'DELETE' }),
  rotateProject: (id: number, kind: 'client_secret' | 'deploy_token' | 'ed25519') =>
    request<{ project: Project }>(`/projects/${id}/rotate`, { method: 'POST', body: JSON.stringify({ kind }) }),

  // releases
  releases: (params: { project_id?: number; platform?: string; channel?: string; q?: string; page?: number; size?: number }) =>
    request<Paged<Release>>(`/releases${query(params)}`),
  createRelease: (form: FormData) => request<{ release: Release; jobs?: DeployJob[] }>('/releases', { method: 'POST', body: form }),
  createReleaseExternal: (payload: Record<string, unknown>) =>
    request<{ release: Release }>('/releases/json', { method: 'POST', body: JSON.stringify(payload) }),
  updateRelease: (id: number, payload: Record<string, unknown>) =>
    request<{ release: Release }>(`/releases/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  deleteRelease: (id: number) => request<{ status: string }>(`/releases/${id}`, { method: 'DELETE' }),
  deployRelease: (id: number, targets: { target_id: number; deploy_dir?: string }[], message = '') =>
    request<{ jobs: DeployJob[]; count: number }>(`/releases/${id}/deploy`, {
      method: 'POST',
      body: JSON.stringify({ targets, message }),
    }),

  // targets
  targets: (project_id?: number) => request<{ items: Target[] }>(`/targets${query({ project_id })}`),
  createTarget: (payload: Partial<Target>) =>
    request<{ target: Target }>('/targets', { method: 'POST', body: JSON.stringify(payload) }),
  updateTarget: (id: number, payload: Partial<Target>) =>
    request<{ target: Target }>(`/targets/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  deleteTarget: (id: number) => request<{ status: string }>(`/targets/${id}`, { method: 'DELETE' }),
  rotateTarget: (id: number) => request<{ target: Target }>(`/targets/${id}/rotate-token`, { method: 'POST' }),
  deployTarget: (id: number, payload: { release_id: number; deploy_dir?: string; message?: string }) =>
    request<{ job: DeployJob }>(`/targets/${id}/deploy`, { method: 'POST', body: JSON.stringify(payload) }),
  deployConfigs: (params: { target_id?: number; project_id?: number }) =>
    request<{ items: DeployConfig[] }>(`/deploy-configs${query(params)}`),
  updateTargetApp: (targetID: number, projectID: number, payload: Record<string, unknown>) =>
    request<{ app: TargetApp }>(`/targets/${targetID}/apps/${projectID}`, {
      method: 'PATCH',
      body: JSON.stringify(payload),
    }),
  deleteTargetApp: (targetID: number, projectID: number) =>
    request<{ status: string }>(`/targets/${targetID}/apps/${projectID}`, { method: 'DELETE' }),

  // jobs
  jobs: (params: { project_id?: number; target_id?: number; status?: string; q?: string; page?: number; size?: number }) =>
    request<Paged<DeployJob>>(`/jobs${query(params)}`),
  job: (id: number) => request<{ job: DeployJob; logs: DeployLog[] }>(`/jobs/${id}`),
  jobLogs: (id: number, after_seq = 0) => request<{ items: DeployLog[] }>(`/jobs/${id}/logs${query({ after_seq })}`),
  cancelJob: (id: number) => request<{ status: string }>(`/jobs/${id}/cancel`, { method: 'POST' }),
  retryJob: (id: number) => request<{ job: DeployJob }>(`/jobs/${id}/retry`, { method: 'POST' }),
  rollbackJob: (id: number) => request<{ job: DeployJob }>(`/jobs/${id}/rollback`, { method: 'POST' }),

  // policies
  policies: (project_id?: number) => request<{ items: VersionPolicy[] }>(`/policies${query({ project_id })}`),
  savePolicy: (payload: Record<string, unknown>) =>
    request<{ policy: VersionPolicy }>('/policies', { method: 'PUT', body: JSON.stringify(payload) }),
  deletePolicy: (id: number) => request<{ status: string }>(`/policies/${id}`, { method: 'DELETE' }),
  blockedVersions: (project_id?: number) =>
    request<{ items: BlockedVersion[] }>(`/blocked-versions${query({ project_id })}`),
  createBlocked: (payload: Record<string, unknown>) =>
    request<{ item: BlockedVersion }>('/blocked-versions', { method: 'POST', body: JSON.stringify(payload) }),
  updateBlocked: (id: number, payload: Record<string, unknown>) =>
    request<{ item: BlockedVersion }>(`/blocked-versions/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  deleteBlocked: (id: number) => request<{ status: string }>(`/blocked-versions/${id}`, { method: 'DELETE' }),

  // settings
  settings: () => request<ServerSettings>('/settings'),
  saveSettings: (public_base_url: string) =>
    request<{ public_base_url: string }>('/settings', { method: 'PUT', body: JSON.stringify({ public_base_url }) }),
  auditLogs: () => request<Paged<AuditLog>>('/audit-logs?size=20'),
}
