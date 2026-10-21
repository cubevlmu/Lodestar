import CloudUpload from '@primeicons/vue/cloud-upload'
import Cog from '@primeicons/vue/cog'
import Gauge from '@primeicons/vue/gauge'
import History from '@primeicons/vue/history'
import Server from '@primeicons/vue/server'
import Shield from '@primeicons/vue/shield'
import Wrench from '@primeicons/vue/wrench'
import Box from '@primeicons/vue/box'
import type { Component } from 'vue'

export type NavKey = 'overview' | 'projects' | 'releases' | 'policies' | 'targets' | 'configs' | 'deployments' | 'settings'

export interface NavItem {
  key: NavKey
  label: string
  description: string
  route: string
  icon: Component
}

export interface NavGroup {
  key: string
  label?: string
  items: NavItem[]
}

export const navItems: NavItem[] = [
  { key: 'overview', label: '总览', description: '发布、目标主机与部署任务的实时状态。', route: '/app/overview', icon: Gauge },
  { key: 'projects', label: '项目', description: '管理应用标识、签名密钥与 CI 部署凭据。', route: '/app/projects', icon: Box },
  { key: 'releases', label: '版本包', description: '登记与上传构建产物，控制灰度与强制更新。', route: '/app/releases', icon: CloudUpload },
  { key: 'policies', label: '更新策略', description: '维护最低版本、停机维护与版本封禁。', route: '/app/policies', icon: Shield },
  { key: 'targets', label: '目标主机', description: '登记生产服务器，查看每台机器上跑了哪些应用。', route: '/app/targets', icon: Server },
  { key: 'configs', label: '部署配置', description: '按「主机 × 应用」设置部署路径、重启命令与白名单。', route: '/app/configs', icon: Wrench },
  { key: 'deployments', label: '部署记录', description: '查看每次部署的日志、耗时与回滚结果。', route: '/app/deployments', icon: History },
  { key: 'settings', label: '设置', description: '账户安全、服务信息与接入指引。', route: '/app/settings', icon: Cog },
]

export const navGroups: NavGroup[] = [
  { key: 'workspace', label: '工作台', items: navItems.filter(item => item.key === 'overview') },
  { key: 'delivery', label: '发布', items: navItems.filter(item => ['projects', 'releases', 'policies'].includes(item.key)) },
  { key: 'production', label: '生产', items: navItems.filter(item => ['targets', 'configs', 'deployments'].includes(item.key)) },
  { key: 'system', label: '系统', items: navItems.filter(item => item.key === 'settings') },
]

export function navItemForPath(path: string): NavItem | undefined {
  return navItems.find(item => path === item.route || path.startsWith(`${item.route}/`))
}

export function navItemForKey(key: NavKey): NavItem | undefined {
  return navItems.find(item => item.key === key)
}
