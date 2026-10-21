<script setup lang="ts">
import LogoMark from './LogoMark.vue'

withDefaults(
  defineProps<{
    tone?: 'default' | 'inverse'
    /** Hide the wordmark, e.g. on the collapsed icon rail. */
    compact?: boolean
  }>(),
  { tone: 'default', compact: false },
)
</script>

<template>
  <div class="flex min-w-0 items-center gap-2.5" :class="compact && 'justify-center'">
    <LogoMark :size="compact ? 30 : 32" />
    <div v-if="!compact" class="min-w-0 leading-none">
      <div class="truncate text-sm font-semibold" :class="tone === 'inverse' ? 'text-white' : 'text-color'">Lodestar</div>
      <div class="mt-1 truncate text-[11px] font-medium" :class="tone === 'inverse' ? 'text-white/70' : 'text-muted-color'">
        发布与部署控制台
      </div>
    </div>
  </div>
</template>
