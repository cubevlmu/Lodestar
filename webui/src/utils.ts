export function formatBytes(bytes: number): string {
  if (!bytes || bytes < 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index++
  }
  return `${value >= 100 || index === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[index]}`
}

export function formatDateTime(value: string | null | undefined): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

export function formatRelative(value: string | null | undefined): string {
  if (!value) return '从未'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  const diff = Date.now() - date.getTime()
  if (diff < 0) return '刚刚'
  const seconds = Math.floor(diff / 1000)
  if (seconds < 45) return '刚刚'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} 分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时前`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days} 天前`
  return formatDateTime(value)
}

export function formatDuration(ms: number | null | undefined): string {
  if (!ms || ms <= 0) return '—'
  if (ms < 1000) return `${ms} ms`
  const seconds = ms / 1000
  if (seconds < 60) return `${seconds.toFixed(1)} s`
  const minutes = Math.floor(seconds / 60)
  const rest = Math.round(seconds % 60)
  return `${minutes}m ${rest}s`
}

export function codeToVersion(code: number): string {
  if (!code || code <= 0) return '—'
  const major = Math.floor(code / 1_000_000)
  const minor = Math.floor(code / 1000) % 1000
  const patch = code % 1000
  return `${major}.${minor}.${patch}`
}

export function formatUptime(seconds: number): string {
  if (!seconds || seconds < 0) return '—'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days} 天 ${hours} 小时`
  if (hours > 0) return `${hours} 小时 ${minutes} 分钟`
  return `${minutes} 分钟`
}

const platformLabels: Record<string, string> = {
  windows: 'Windows',
  linux: 'Linux',
  darwin: 'macOS',
  macos: 'macOS',
  android: 'Android',
  ios: 'iOS',
}

export function platformLabel(value: string): string {
  return platformLabels[value] || value || '—'
}

export type Severity = 'success' | 'info' | 'warn' | 'danger' | 'secondary' | 'contrast'

export function jobStatusMeta(status: string): { label: string; severity: Severity; icon: string } {
  switch (status) {
    case 'success':
      return { label: '成功', severity: 'success', icon: 'pi pi-check-circle' }
    case 'running':
      return { label: '执行中', severity: 'info', icon: 'pi pi-spin pi-spinner' }
    case 'pending':
      return { label: '排队中', severity: 'warn', icon: 'pi pi-clock' }
    case 'failed':
      return { label: '失败', severity: 'danger', icon: 'pi pi-times-circle' }
    case 'canceled':
      return { label: '已取消', severity: 'secondary', icon: 'pi pi-ban' }
    default:
      return { label: status || '未知', severity: 'secondary', icon: 'pi pi-question-circle' }
  }
}

export function targetStatusMeta(status: string): { label: string; severity: Severity; color: string } {
  switch (status) {
    case 'online':
      return { label: '在线', severity: 'success', color: 'var(--p-green-500)' }
    case 'deploying':
      return { label: '部署中', severity: 'info', color: 'var(--p-blue-500)' }
    case 'error':
      return { label: '异常', severity: 'danger', color: 'var(--p-red-500)' }
    default:
      return { label: '离线', severity: 'secondary', color: 'var(--p-surface-400)' }
  }
}

export function triggerLabel(trigger: string): string {
  switch (trigger) {
    case 'ci':
      return 'CI'
    case 'manual':
      return '手动'
    case 'retry':
      return '重试'
    case 'rollback':
      return '回滚'
    default:
      return trigger || '—'
  }
}

export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    try {
      const area = document.createElement('textarea')
      area.value = text
      area.style.position = 'fixed'
      area.style.opacity = '0'
      document.body.appendChild(area)
      area.select()
      document.execCommand('copy')
      document.body.removeChild(area)
      return true
    } catch {
      return false
    }
  }
}

// Builds pass the tag as-is ("v0.1.0"), so templates that prefix a "v" would
// render "vv0.1.0". Normalise to exactly one leading v.
export function formatVersion(value: string | null | undefined): string {
  const raw = (value ?? '').trim()
  if (!raw) return ''
  return `v${raw.replace(/^v+/i, '')}`
}
