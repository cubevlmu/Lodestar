<script setup lang="ts">
import Skeleton from 'primevue/skeleton'

withDefaults(defineProps<{ cards?: number }>(), { cards: 4 })
</script>

<template>
  <div class="space-y-4" role="status" aria-label="正在加载页面">
    <div class="grid grid-cols-2 gap-3 xl:grid-cols-4">
      <div v-for="index in cards" :key="index" class="surface-panel rounded-lg p-4">
        <Skeleton width="58%" height=".75rem" />
        <Skeleton class="mt-3" width="42%" height="1.75rem" />
      </div>
    </div>
    <div class="surface-panel rounded-lg p-4">
      <Skeleton width="8rem" height="1rem" />
      <Skeleton class="mt-4" width="100%" height="10rem" />
    </div>
  </div>
</template>
