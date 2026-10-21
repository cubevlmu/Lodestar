<script setup lang="ts">
import { useId } from 'vue'

/**
 * Lodestar mark: a four-point pole star on a rounded plate.
 * The mark reads as a navigation star, which is what the product is named after.
 */
withDefaults(
  defineProps<{
    /** Rendered size in px (square). */
    size?: number
    /** Draw the gradient plate behind the star. */
    plate?: boolean
  }>(),
  { size: 32, plate: true },
)

const uid = useId()
const gradientId = `lodestar-gradient-${uid}`
const STAR = 'M24 6.4C25.5 17.5 30.5 22.5 41.6 24 30.5 25.5 25.5 30.5 24 41.6 22.5 30.5 17.5 25.5 6.4 24 17.5 22.5 22.5 17.5 24 6.4Z'
</script>

<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 48 48"
    fill="none"
    role="img"
    aria-label="Lodestar"
    class="shrink-0"
  >
    <defs>
      <linearGradient :id="gradientId" x1="4" y1="2" x2="44" y2="46" gradientUnits="userSpaceOnUse">
        <stop offset="0" stop-color="#818cf8" />
        <stop offset="0.5" stop-color="#4f46e5" />
        <stop offset="1" stop-color="#0ea5e9" />
      </linearGradient>
    </defs>

    <template v-if="plate">
      <rect width="48" height="48" rx="12" :fill="`url(#${gradientId})`" />
      <circle cx="24" cy="24" r="15.6" stroke="#ffffff" stroke-opacity="0.22" stroke-width="1.2" />
      <path :d="STAR" fill="#ffffff" />
      <circle cx="24" cy="24" r="1.5" :fill="`url(#${gradientId})`" fill-opacity="0.9" />
    </template>

    <template v-else>
      <path :d="STAR" :fill="`url(#${gradientId})`" />
    </template>
  </svg>
</template>
