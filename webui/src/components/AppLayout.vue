<script setup lang="ts">
import Refresh from '@primeicons/vue/refresh'
import SidebarIcon from '@primeicons/vue/sidebar'
import Button from 'primevue/button'
import SidebarTrigger from 'primevue/sidebartrigger'
import Toolbar from 'primevue/toolbar'
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppNavigation from './AppNavigation.vue'
import ThemeToggle from './ThemeToggle.vue'
import { navItemForKey, navItemForPath, type NavKey } from '../navigation'
import { api, clearToken } from '../services/api'
import { formatVersion } from '../utils'
import type { User } from '../types'

const route = useRoute()
const router = useRouter()

const profile = ref<User | null>(null)
const profileLoaded = ref(false)
const serverVersion = ref('')
const refreshCounter = ref(0)
const currentYear = new Date().getFullYear()

const currentItem = computed(() => navItemForPath(route.path) ?? navItemForPath('/app/overview')!)
const currentKey = computed<NavKey>(() => currentItem.value.key)

function navigate(key: NavKey) {
  const item = navItemForKey(key)
  if (item) router.push(item.route)
}

async function logout() {
  try {
    await api.logout()
  } catch {
    /* the session may already be gone */
  }
  clearToken()
  await router.push('/login')
}

function refresh() {
  refreshCounter.value++
}

onMounted(async () => {
  try {
    const response = await api.me()
    profile.value = response.user
  } catch {
    /* the API client redirects on 401 */
  } finally {
    profileLoaded.value = true
  }
  try {
    const settings = await api.settings()
    serverVersion.value = settings.version
  } catch {
    serverVersion.value = ''
  }
})
</script>

<template>
  <AppNavigation
    :model-value="currentKey"
    :profile="profile"
    :profile-loaded="profileLoaded"
    @select="navigate"
    @logout="logout"
  >
    <div class="flex min-h-full min-w-0 flex-1 flex-col">
      <Toolbar class="shrink-0 !rounded-none border-x-0 border-t-0 px-3 sm:px-7">
        <template #start>
          <div class="flex items-center gap-2">
            <SidebarTrigger
              target="lodestar-menu"
              severity="secondary"
              :text="true"
              size="small"
              class="!-ml-2 !h-8 !min-h-8 !w-8 !rounded-md !p-0"
              aria-label="切换菜单"
            >
              <SidebarIcon />
            </SidebarTrigger>
            <span
              class="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-primary-100 text-primary-700 dark:bg-primary-900/50 dark:text-primary-300"
              aria-hidden="true"
            >
              <component :is="currentItem.icon" :size="15" />
            </span>
            <span class="truncate text-sm font-bold text-color">{{ currentItem.label }}</span>
          </div>
        </template>
        <template #end>
          <div class="flex items-center gap-0.5">
            <Button
              v-tooltip.bottom="'刷新数据'"
              severity="secondary"
              text
              size="small"
              class="!h-8 !min-h-8 !w-8 !rounded-md !p-0"
              aria-label="刷新数据"
              @click="refresh"
            >
              <Refresh :size="16" />
            </Button>
            <ThemeToggle />
          </div>
        </template>
      </Toolbar>

      <main class="mx-auto w-full max-w-[1480px] flex-1 px-3 pb-8 pt-4 sm:px-7 sm:pb-10 sm:pt-6">
        <section class="mb-4 flex flex-col justify-between gap-3 sm:mb-6 sm:flex-row sm:items-end">
          <div class="min-w-0">
            <h1 class="page-title">{{ currentItem.label }}</h1>
            <p class="mt-1 text-[13px] leading-5 text-muted-color sm:text-sm">{{ currentItem.description }}</p>
          </div>
          <div id="page-actions" class="flex shrink-0 flex-wrap items-center gap-2"></div>
        </section>

        <RouterView v-slot="{ Component }">
          <component :is="Component" :key="`${route.path}-${refreshCounter}`" />
        </RouterView>
      </main>

      <footer class="mx-auto flex w-full max-w-[1480px] flex-wrap items-center justify-center gap-x-3 gap-y-1 px-3 py-4 pb-[max(1rem,env(safe-area-inset-bottom))] text-center text-[11px] text-muted-color sm:px-7 sm:text-xs">
        <span>© 2026 cubevlmu(@//khfahqp/)</span>
        <span v-if="serverVersion" class="mono">{{ formatVersion(serverVersion) }}</span>
      </footer>
    </div>
  </AppNavigation>
</template>
