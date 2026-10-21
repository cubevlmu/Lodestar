<script setup lang="ts">
import Moon from '@primeicons/vue/moon'
import Sun from '@primeicons/vue/sun'
import Button from 'primevue/button'
import { computed, ref } from 'vue'

const dark = ref(document.documentElement.classList.contains('dark'))
const label = computed(() => (dark.value ? '切换到浅色模式' : '切换到深色模式'))

function toggle() {
  dark.value = !dark.value
  document.documentElement.classList.toggle('dark', dark.value)
  localStorage.setItem('lodestar-theme', dark.value ? 'dark' : 'light')
}
</script>

<template>
  <Button
    severity="secondary"
    text
    size="small"
    class="!h-8 !min-h-8 !w-8 !rounded-md !p-0"
    :aria-label="label"
    :title="label"
    @click="toggle"
  >
    <Sun v-if="dark" :size="17" />
    <Moon v-else :size="17" />
  </Button>
</template>
