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
 *
 * `source` says how the number was obtained. A probe measures the upstream
 * itself; `traffic` is measured on the requests users actually made, so the
 * tooltip spells that out (and how many requests it rests on) instead of
 * letting the reader assume a continuous monitor.
 */
const props = withDefaults(
  defineProps<{
    status: string | null | undefined
    uptime?: number | null
    showLabel?: boolean
    source?: string | null
    requests?: number | null
  }>(),
  {
    uptime: null,
    showLabel: false,
    source: null,
    requests: null
  }
)

const { t } = useI18n()

const meta = computed(() => modelHealthStatusMeta(props.status))

const formatUptime = (value: number) => `${Number(value).toFixed(2)}%`

const tooltip = computed(() => {
  const parts: string[] = []
  if (props.uptime === null || props.uptime === undefined) {
    parts.push(t(meta.value.labelKey))
  } else {
    parts.push(`${t(meta.value.labelKey)} · ${formatUptime(props.uptime)} ${t('modelHealth.uptime')}`)
  }
  if (props.source === 'traffic') {
    const count = Number(props.requests ?? 0)
    parts.push(
      count > 0
        ? `${t('modelHealth.sourceTraffic')} · ${t('modelHealth.trafficSample', { count })}`
        : t('modelHealth.sourceTraffic')
    )
  }
  return parts.join(' · ')
})
</script>
