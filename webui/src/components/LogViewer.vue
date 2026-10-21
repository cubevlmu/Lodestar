<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import type { DeployLog } from '../types'

const props = withDefaults(defineProps<{ logs: DeployLog[]; autoscroll?: boolean }>(), { autoscroll: true })
const box = ref<HTMLElement | null>(null)

watch(
  () => props.logs.length,
  async () => {
    if (!props.autoscroll) return
    await nextTick()
    if (box.value) box.value.scrollTop = box.value.scrollHeight
  },
)
</script>

<template>
  <div ref="box" class="log-view">
    <div v-if="!props.logs.length" class="text-surface-500">等待日志输出…</div>
    <div v-for="log in props.logs" :key="log.seq" class="log-line" :class="`log-${log.stream}`">
      <span class="log-seq">{{ log.seq }}</span>
      <span>{{ log.line }}</span>
    </div>
  </div>
</template>
