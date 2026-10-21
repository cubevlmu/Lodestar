export interface User {
  id: number
  username: string
  display_name: string
  role: string
  last_login_at: string | null
}

export interface Project {
  id: number
  app_id: string
  name: string
  description: string
  repo_url: string
  ed25519_public_key: string
  enabled: boolean
  created_at: string
  updated_at: string
  client_secret?: string
  deploy_token?: string
  release_count?: number
  target_count?: number
  online_targets?: number
  latest_version?: string
}

export interface PackageInfo {
  id: number
  release_id: number
  file_name: string
  storage_path: string
  external_url: string
  size: number
  sha256: string
  created_at: string
}

export interface Release {
  id: number
  project_id: number
  version: string
  version_code: number
  platform: string
  arch: string
  channel: string
  title: string
  changelog: string
  min_supported_code: number
  force: boolean
  enabled: boolean
  rollout_percent: number
  source: string
  created_by: string
  created_at: string
  updated_at: string
  package: PackageInfo
}

export type TargetStatus = 'online' | 'offline' | 'deploying' | 'error'

export interface TargetApp {
  id: number
  target_id: number
  project_id: number
  deploy_dir: string
  backup_dir: string
  stop_command: string
  start_command: string
  restart_command: string
  health_check_url: string
  preserve: string
  backup_keep: number
  current_version: string
  current_code: number
  last_deploy_at: string | null
  project_name: string
  project_app_id: string
}

export interface Target {
  id: number
  name: string
  platform: string
  arch: string
  channel: string
  status: TargetStatus
  current_version: string
  current_code: number
  hostname: string
  os: string
  agent_version: string
  deploy_root: string
  backup_dir: string
  auto_deploy: boolean
  notes: string
  last_seen_at: string | null
  last_deploy_at: string | null
  created_at: string
  updated_at: string
  token?: string
  apps: TargetApp[]
}

export type JobStatus = 'pending' | 'running' | 'success' | 'failed' | 'canceled'

// One "this project on this host" record — what the 部署配置 page edits.
export interface DeployConfig extends TargetApp {
  target_name: string
  target_platform: string
  target_status: TargetStatus
  deploy_root: string
  project_name: string
  project_app_id: string
}

export interface DeployJob {
  id: number
  project_id: number
  release_id: number
  target_id: number
  version: string
  version_code: number
  mode: 'deploy' | 'rollback'
  status: JobStatus
  trigger: string
  requested_by: string
  message: string
  error: string
  backup_path: string
  deploy_dir: string
  backup_dir: string
  rolled_back: boolean
  duration_ms: number
  started_at: string | null
  finished_at: string | null
  created_at: string
  target?: Target
}

export interface DeployLog {
  id: number
  job_id: number
  seq: number
  stream: 'info' | 'stdout' | 'stderr' | 'error'
  line: string
  created_at: string
}

export interface VersionPolicy {
  id: number
  project_id: number
  platform: string
  channel: string
  min_allowed_code: number
  message: string
  maintenance: boolean
  maintenance_msg: string
  created_at: string
  updated_at: string
}

export interface BlockedVersion {
  id: number
  project_id: number
  platform: string
  channel: string
  version_code: number
  reason: string
  enabled: boolean
  created_at: string
}

export interface DailyCount {
  day: string
  total: number
  success: number
  failed: number
}

export interface Overview {
  projects: number
  releases: number
  releases_week: number
  targets: number
  targets_online: number
  jobs_today: number
  jobs_week: number
  jobs_success_week: number
  jobs_failed_week: number
  success_rate: number
  avg_duration_ms: number
  storage_bytes: number
  recent_jobs: DeployJob[]
  targets_list: Target[]
  daily: DailyCount[]
  server: { version: string; commit: string; uptime: number; started_at: string }
}

export interface ServerSettings {
  version: string
  commit: string
  started_at: string
  uptime: number
  public_base_url: string
  effective_base_url: string
  storage_dir: string
  db_path: string
  data_dir: string
  session_ttl_hours: number
  max_upload_bytes: number
  package_count: number
  release_count: number
  job_count: number
}

export interface AuditLog {
  id: number
  user_id: number
  username: string
  action: string
  detail: string
  ip: string
  created_at: string
}

export interface Paged<T> {
  items: T[]
  total: number
  page: number
  size: number
}
