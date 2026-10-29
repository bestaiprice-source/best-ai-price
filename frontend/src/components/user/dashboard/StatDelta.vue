<template>
  <span
    v-if="delta"
    class="inline-flex items-center gap-1 font-mono text-[11px] font-semibold"
    :class="toneClass"
    :title="t('ui.userDashboardStats.vsYesterday')"
  >
    <span aria-hidden="true">{{ arrow }}</span>
    <span>{{ label }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TrendDelta } from '@/utils/dashboardTrend'

/**
 * "vs yesterday" delta badge for a stat card.
 *
 * `inverse` flips the colour semantics for cost-like metrics, where a rising
 * number is spending more rather than doing more.
 */
const props = withDefaults(
  defineProps<{
    delta: TrendDelta | null
    format?: (value: number) => string
    inverse?: boolean
  }>(),
  {
    inverse: false
  }
)

const { t } = useI18n()

const arrow = computed(() => {
  if (!props.delta) return ''
  if (props.delta.direction === 'up') return '▲'
  if (props.delta.direction === 'down') return '▼'
  return '■'
})

const label = computed(() => {
  const delta = props.delta
  if (!delta) return ''
  if (delta.percent !== null) return `${Math.abs(delta.percent).toFixed(1)}%`
  // The previous day was zero, so a percentage is undefined — show the raw move.
  const absolute = props.format ? props.format(Math.abs(delta.absolute)) : String(Math.abs(delta.absolute))
  return `+${absolute}`
})

const toneClass = computed(() => {
  const direction = props.delta?.direction
  if (!direction || direction === 'flat') return 'text-gray-400 dark:text-gray-500'
  const positive = props.inverse ? direction === 'down' : direction === 'up'
  return positive ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'
})
</script>
