<script setup lang="ts">
import Tag from 'primevue/tag'
import { computed } from 'vue'
import { jobStatusMeta, targetStatusMeta } from '../utils'

const props = withDefaults(defineProps<{ status: string; kind?: 'job' | 'target' }>(), { kind: 'job' })

const meta = computed(() => (props.kind === 'target' ? targetStatusMeta(props.status) : jobStatusMeta(props.status)))
</script>

<template>
  <Tag :value="meta.label" :severity="meta.severity" :icon="'icon' in meta ? meta.icon : undefined" class="!px-2 !py-0.5 !text-xs" />
</template>
