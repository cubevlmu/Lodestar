<script setup lang="ts">
import Avatar from 'primevue/avatar'
import Menu from 'primevue/menu'
import Sidebar from 'primevue/sidebar'
import SidebarAside from 'primevue/sidebaraside'
import SidebarBackdrop from 'primevue/sidebarbackdrop'
import SidebarContent from 'primevue/sidebarcontent'
import SidebarFooter from 'primevue/sidebarfooter'
import SidebarGroup from 'primevue/sidebargroup'
import SidebarGroupContent from 'primevue/sidebargroupcontent'
import SidebarGroupLabel from 'primevue/sidebargrouplabel'
import SidebarHeader from 'primevue/sidebarheader'
import SidebarLayout from 'primevue/sidebarlayout'
import SidebarMain from 'primevue/sidebarmain'
import SidebarMenu from 'primevue/sidebarmenu'
import SidebarMenuButton from 'primevue/sidebarmenubutton'
import SidebarMenuItem from 'primevue/sidebarmenuitem'
import SidebarPanel from 'primevue/sidebarpanel'
import SidebarRail from 'primevue/sidebarrail'
import SidebarSpacer from 'primevue/sidebarspacer'
import Skeleton from 'primevue/skeleton'
import ChevronDown from '@primeicons/vue/chevron-down'
import Cog from '@primeicons/vue/cog'
import SignOut from '@primeicons/vue/sign-out'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import BrandMark from './BrandMark.vue'
import { navGroups, type NavKey } from '../navigation'
import type { User } from '../types'

const props = withDefaults(
  defineProps<{
    modelValue: NavKey
    profile?: User | null
    profileLoaded?: boolean
  }>(),
  { profile: null, profileLoaded: false },
)

const emit = defineEmits<{
  select: [key: NavKey]
  logout: []
}>()

const open = ref(true)
const isMobile = ref(false)
const userMenu = ref<InstanceType<typeof Menu> | null>(null)

const profileName = computed(() => props.profile?.display_name?.trim() || props.profile?.username || '未登录')
const profileRole = computed(() => (props.profile?.role === 'admin' ? '管理员' : '操作员'))
const avatarLabel = computed(() => profileName.value.slice(0, 1).toUpperCase())

const userMenuItems = computed(() => [
  { label: '设置', icon: Cog, command: () => emit('select', 'settings') },
  { separator: true },
  { label: '退出登录', icon: SignOut, command: () => emit('logout') },
])

let mediaQuery: MediaQueryList | null = null
let onMediaQueryChange: ((event: MediaQueryListEvent) => void) | null = null

if (typeof window !== 'undefined') {
  isMobile.value = window.matchMedia('(max-width: 1023px)').matches
  open.value = !isMobile.value
}

function select(key: NavKey) {
  if (isMobile.value) open.value = false
  emit('select', key)
}

function toggleUserMenu(event: Event) {
  userMenu.value?.toggle(event)
}

onMounted(() => {
  mediaQuery = window.matchMedia('(max-width: 1023px)')
  isMobile.value = mediaQuery.matches
  open.value = !mediaQuery.matches
  onMediaQueryChange = event => {
    isMobile.value = event.matches
    open.value = !event.matches
  }
  mediaQuery.addEventListener('change', onMediaQueryChange)
})

onBeforeUnmount(() => {
  if (mediaQuery && onMediaQueryChange) mediaQuery.removeEventListener('change', onMediaQueryChange)
})
</script>

<template>
  <SidebarLayout class="relative h-screen overflow-hidden">
    <SidebarBackdrop v-if="isMobile && open" class="!absolute" />
    <Sidebar id="lodestar-menu" :collapsible="isMobile ? 'offcanvas' : 'icon'" :overlay="isMobile" v-model:open="open">
      <SidebarSpacer />
      <SidebarAside>
        <SidebarPanel>
          <SidebarHeader>
            <div class="app-sidebar-brand-button px-1 py-2">
              <BrandMark :compact="!open" />
            </div>
          </SidebarHeader>

          <SidebarContent>
            <template v-if="props.profileLoaded">
              <SidebarGroup v-for="group in navGroups" :key="group.key">
                <SidebarGroupLabel v-if="group.label">{{ group.label }}</SidebarGroupLabel>
                <SidebarGroupContent>
                  <SidebarMenu :aria-label="group.label">
                    <SidebarMenuItem v-for="item in group.items" :key="item.key">
                      <SidebarMenuButton
                        :is-active="props.modelValue === item.key"
                        :title="item.label"
                        :aria-current="props.modelValue === item.key ? 'page' : undefined"
                        @click="select(item.key)"
                      >
                        <component :is="item.icon" :size="18" aria-hidden="true" />
                        <span>{{ item.label }}</span>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            </template>
            <div v-else class="space-y-2 p-2" aria-label="正在加载主导航">
              <Skeleton v-for="index in 6" :key="index" height="2rem" border-radius="0.375rem" />
            </div>
          </SidebarContent>

          <SidebarFooter>
            <SidebarMenu aria-label="账户操作">
              <SidebarMenuItem>
                <SidebarMenuButton class="app-sidebar-profile !p-2" aria-haspopup="true" aria-controls="lodestar-user-menu" @click="toggleUserMenu">
                  <Skeleton v-if="!props.profileLoaded" shape="circle" size="1.5rem" />
                  <Avatar v-else :label="avatarLabel" shape="circle" class="size-6 shrink-0 text-xs" />
                  <span class="min-w-0 flex-1 truncate text-sm font-medium">{{ profileName }}</span>
                  <ChevronDown class="ml-auto" :size="16" aria-hidden="true" />
                </SidebarMenuButton>
                <Menu ref="userMenu" id="lodestar-user-menu" :model="userMenuItems" :popup="true">
                  <template #start>
                    <div class="px-3 py-1 text-xs font-medium text-muted-color">{{ profileRole }}</div>
                  </template>
                </Menu>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarFooter>
          <SidebarRail />
        </SidebarPanel>
      </SidebarAside>
    </Sidebar>
    <SidebarMain class="flex min-h-0 min-w-0 flex-1 flex-col overflow-y-auto">
      <slot />
    </SidebarMain>
  </SidebarLayout>
</template>
