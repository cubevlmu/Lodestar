import { createRouter, createWebHistory } from 'vue-router'
import { getToken } from './services/api'

const AppLayout = () => import('./components/AppLayout.vue')
const LoginView = () => import('./views/LoginView.vue')
const OverviewView = () => import('./views/OverviewView.vue')
const ProjectsView = () => import('./views/ProjectsView.vue')
const ReleasesView = () => import('./views/ReleasesView.vue')
const TargetsView = () => import('./views/TargetsView.vue')
const DeployConfigsView = () => import('./views/DeployConfigsView.vue')
const DeploymentsView = () => import('./views/DeploymentsView.vue')
const PoliciesView = () => import('./views/PoliciesView.vue')
const SettingsView = () => import('./views/SettingsView.vue')

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: () => (getToken() ? '/app/overview' : '/login') },
    { path: '/login', component: LoginView, meta: { public: true } },
    {
      path: '/app',
      component: AppLayout,
      children: [
        { path: '', redirect: '/app/overview' },
        { path: 'overview', component: OverviewView },
        { path: 'projects', component: ProjectsView },
        { path: 'releases', component: ReleasesView },
        { path: 'policies', component: PoliciesView },
        { path: 'targets', component: TargetsView },
        { path: 'configs', component: DeployConfigsView },
        { path: 'deployments', component: DeploymentsView },
        { path: 'settings', component: SettingsView },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.beforeEach(to => {
  const hasSession = Boolean(getToken())
  if (to.meta.public) {
    if (hasSession && to.path === '/login') return '/app/overview'
    return true
  }
  if (!hasSession) return '/login'
  return true
})

export default router
