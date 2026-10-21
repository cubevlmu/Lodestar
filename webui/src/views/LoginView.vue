<script setup lang="ts">
import ArrowRight from '@primeicons/vue/arrow-right'
import User from '@primeicons/vue/user'
import Button from 'primevue/button'
import InputGroup from 'primevue/inputgroup'
import InputGroupAddon from 'primevue/inputgroupaddon'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import LogoMark from '../components/LogoMark.vue'
import ThemeToggle from '../components/ThemeToggle.vue'
import { api, setToken } from '../services/api'

const router = useRouter()
const username = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')
const year = new Date().getFullYear()

onMounted(() => {
  const input = document.getElementById('login-username') as HTMLInputElement | null
  input?.focus()
})

async function submit() {
  error.value = ''
  if (!username.value.trim() || !password.value) {
    error.value = '请输入用户名和密码。'
    return
  }
  loading.value = true
  try {
    const response = await api.login(username.value.trim(), password.value)
    setToken(response.token)
    await router.push('/app/overview')
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '登录失败，请稍后重试'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="auth-shell">
    <div class="auth-glow" aria-hidden="true"></div>

    <header class="relative z-10 flex items-center justify-end px-4 pt-4 sm:px-6 sm:pt-5">
      <ThemeToggle />
    </header>

    <div class="relative z-10 flex flex-1 items-center justify-center px-4 py-8 sm:px-6">
      <div class="w-full max-w-[22rem] sm:max-w-sm">
        <div class="mb-7 flex flex-col items-center text-center">
          <LogoMark :size="52" />
          <h1 class="mt-4 text-xl font-semibold text-color sm:text-2xl">Lodestar</h1>
          <p class="mt-1.5 text-xs font-medium text-muted-color sm:text-sm">发布与部署控制台</p>
        </div>

        <div class="surface-panel rounded-xl p-5 sm:p-6">
          <form class="flex flex-col gap-4" @submit.prevent="submit">
            <div class="flex flex-col gap-2">
              <label class="text-sm font-medium text-color" for="login-username">用户名</label>
              <InputGroup>
                <InputGroupAddon><User /></InputGroupAddon>
                <InputText id="login-username" v-model="username" autocomplete="username" placeholder="admin" fluid />
              </InputGroup>
            </div>

            <div class="flex flex-col gap-2">
              <label class="text-sm font-medium text-color" for="login-password">密码</label>
              <Password
                input-id="login-password"
                v-model="password"
                :feedback="false"
                toggle-mask
                autocomplete="current-password"
                placeholder="输入密码"
                fluid
                input-class="w-full"
              />
            </div>

            <Message v-if="error" severity="error" size="small">{{ error }}</Message>

            <Button type="submit" class="w-full" :loading="loading">
              登录 <ArrowRight :size="17" />
            </Button>
          </form>

          <p class="mt-4 border-t border-surface-200 pt-4 text-[11px] leading-5 text-muted-color dark:border-surface-800">
            首次启动时，服务端会创建管理员账户并把初始密码写入
            <code class="mono">data/initial-admin.txt</code>，登录后请立即修改。
          </p>
        </div>
      </div>
    </div>

    <footer class="relative z-10 pb-[max(1rem,env(safe-area-inset-bottom))] text-center text-[11px] text-muted-color">
      © 2026 cubevlmu(@//khfahqp/)
    </footer>
  </main>
</template>
