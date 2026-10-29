<template>
  <span class="inline-flex items-center gap-1.5" :title="tooltip">
    <span class="inline-block h-2 w-2 shrink-0 rounded-full" :class="meta.dotClass"></span>
    <span v-if="showLabel" class="text-[11px] font-medium" :class="meta.textClass">
      {{ t(meta.labelKey) }}
    </span>
    <span v-if="uptime !== null" class="font-mono text-[11px] tabular-nums" :class="meta.textClass">
      {{ formatUptime(uptime) }}
    </span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { modelHealthStatusMeta } from '@/constants/modelHealth'

/**
 * Status dot + uptime for one model, with the legend wording in the tooltip.
 *
 * `uptime` is nullable because the backend deliberately omits it for models
 * under planned maintenance and for models with no probe history — showing a
 * `0.00%` there would read as a total outage.
 */
const props = withDefaults(
  defineProps<{
    status: string | null | undefined
    uptime?: number | null
    showLabel?: boolean
  }>(),
  {
    uptime: null,
    showLabel: false
  }
)

const { t } = useI18n()

const meta = computed(() => modelHealthStatusMeta(props.status))

const formatUptime = (value: number) => `${Number(value).toFixed(2)}%`

const tooltip = computed(() => {
  if (props.uptime === null || props.uptime === undefined) return t(meta.value.labelKey)
  return `${t(meta.value.labelKey)} · ${formatUptime(props.uptime)} ${t('modelHealth.uptime')}`
})
</script>
