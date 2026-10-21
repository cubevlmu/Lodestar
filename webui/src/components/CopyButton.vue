<script setup lang="ts">
import Button from 'primevue/button'
import { useToast } from 'primevue/usetoast'
import { copyText } from '../utils'

const props = defineProps<{ value: string; label?: string }>()
const toast = useToast()

async function copy() {
  const ok = await copyText(props.value)
  toast.add({ severity: ok ? 'success' : 'error', summary: ok ? '已复制' : '复制失败', life: 1600 })
}
</script>

<template>
  <Button
    v-tooltip.bottom="'复制'"
    :label="props.label"
    icon="pi pi-copy"
    severity="secondary"
    size="small"
    :text="!props.label"
    :class="props.label ? 'shrink-0' : '!h-7 !min-h-7 !w-7 !rounded-md !p-0'"
    @click="copy"
  />
</template>
