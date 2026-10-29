<template>
  <svg
    v-if="points"
    :width="width"
    :height="height"
    :viewBox="`0 0 ${width} ${height}`"
    preserveAspectRatio="none"
    class="overflow-visible"
    role="img"
    aria-hidden="true"
  >
    <polyline
      :points="points"
      fill="none"
      :stroke="stroke"
      :stroke-width="strokeWidth"
      stroke-linecap="round"
      stroke-linejoin="round"
    />
  </svg>
</template>

<script setup lang="ts">
import { computed } from 'vue'

/**
 * Inline SVG sparkline — no chart library, no extra dependency.
 *
 * Values are plotted oldest → newest across the full width; a flat series
 * (all values equal) renders as a horizontal middle line rather than collapsing
 * into the top edge, and fewer than two finite values renders nothing.
 */
const props = withDefaults(
  defineProps<{
    values: number[]
    width?: number
    height?: number
    stroke?: string
    strokeWidth?: number
  }>(),
  {
    width: 64,
    height: 22,
    stroke: 'currentColor',
    strokeWidth: 1.5
  }
)

const points = computed<string | null>(() => {
  const values = (props.values ?? []).map(Number).filter((v) => Number.isFinite(v))
  if (values.length < 2) return null

  const width = props.width
  const height = props.height
  // Keep the stroke inside the viewBox: a 1.5px line would otherwise be clipped
  // in half at the top and bottom edges.
  const inset = props.strokeWidth
  const usable = Math.max(height - inset * 2, 1)

  const min = Math.min(...values)
  const max = Math.max(...values)
  const step = width / (values.length - 1)

  return values
    .map((value, index) => {
      const x = index * step
      const ratio = max === min ? 0.5 : (value - min) / (max - min)
      const y = inset + (1 - ratio) * usable
      return `${x.toFixed(2)},${y.toFixed(2)}`
    })
    .join(' ')
})
</script>
