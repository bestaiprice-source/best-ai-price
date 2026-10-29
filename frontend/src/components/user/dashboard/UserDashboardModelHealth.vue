<template>
  <div v-if="rows.length" class="bap-card p-5 sm:p-6">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0">
        <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('modelHealth.title') }}
        </h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
          {{ t('modelHealth.subtitle', { window: windowDays }) }}
        </p>
      </div>
      <router-link
        to="/monitor"
        class="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-[#4791ff] hover:underline"
      >
        {{ t('modelHealth.viewAll') }}
        <span aria-hidden="true">&rarr;</span>
      </router-link>
    </div>

    <ModelHealthLegend class="mt-3" />

    <ul class="mt-4 divide-y divide-black/[0.04] dark:divide-white/[0.06]">
      <li
        v-for="row in visibleRows"
        :key="`${row.monitor_id}-${row.model}`"
        class="flex items-center justify-between gap-3 py-2"
      >
        <div class="min-w-0">
          <p class="truncate text-xs font-medium text-gray-900 dark:text-white" :title="row.model">
            {{ row.model }}
          </p>
          <p v-if="row.monitor_name || row.provider" class="truncate text-[11px] text-gray-400">
            {{ row.monitor_name || row.provider }}
          </p>
        </div>
        <ModelHealthBadge :status="row.status" :uptime="row.availability_pct" class="shrink-0" />
      </li>
    </ul>

    <p v-if="hiddenCount > 0" class="mt-3 text-[11px] text-gray-400">
      {{ t('modelHealth.moreModels', { count: hiddenCount }) }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import ModelHealthBadge from '@/components/user/monitor/ModelHealthBadge.vue'
import ModelHealthLegend from '@/components/user/monitor/ModelHealthLegend.vue'
import { useModelHealth } from '@/composables/useModelHealth'

/**
 * Model health block on the dashboard — the operational answer the static
 * "Gateway Operational" pill never gave: per-model uptime over the probe window,
 * worst first, with the shared legend.
 *
 * Renders nothing at all when no monitor is configured (or the channel-monitor
 * feature is off): an empty card would just be noise on the dashboard.
 */
const MAX_ROWS = 6

const { t } = useI18n()
const { items, windowDays, load } = useModelHealth()

onMounted(() => {
  void load()
})

const rows = computed(() =>
  [...items.value].sort((a, b) => {
    const av = a.availability_pct
    const bv = b.availability_pct
    // Models without an uptime (maintenance, no history) go last, then worst
    // uptime first so a degradation is the first thing the operator sees.
    if (av === null && bv === null) return a.model.localeCompare(b.model)
    if (av === null) return 1
    if (bv === null) return -1
    if (av !== bv) return av - bv
    return a.model.localeCompare(b.model)
  })
)

const visibleRows = computed(() => rows.value.slice(0, MAX_ROWS))
const hiddenCount = computed(() => Math.max(rows.value.length - MAX_ROWS, 0))
</script>
