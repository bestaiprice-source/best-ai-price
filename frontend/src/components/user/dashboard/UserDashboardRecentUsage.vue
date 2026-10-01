<template>
  <div class="bap-card p-5">
    <div class="mb-4 flex items-center justify-between border-b border-black/[0.04] pb-3 dark:border-white/[0.06]">
      <div class="flex items-center gap-2">
        <h2 class="text-sm font-semibold tracking-tight text-gray-900 dark:text-white">
          {{ t('dashboard.recentUsage') }}
        </h2>
        <span class="bap-pill bap-pill-blue text-[10px]">
          {{ t('dashboard.last7Days') }}
        </span>
      </div>
      <router-link
        to="/usage"
        class="text-xs font-medium text-[#4791ff] hover:underline"
      >
        {{ t('dashboard.viewAllUsage') }} &rarr;
      </router-link>
    </div>

    <div>
      <div v-if="loading" class="flex items-center justify-center py-10">
        <LoadingSpinner size="md" />
      </div>

      <div v-else-if="data.length === 0" class="py-8">
        <EmptyState :title="t('dashboard.noUsageRecords')" :description="t('dashboard.startUsingApi')" />
      </div>

      <div v-else class="space-y-2.5">
        <div
          v-for="log in data"
          :key="log.id"
          class="flex items-center justify-between rounded-xl border border-black/[0.04] bg-white p-3.5 shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:border-black/15 hover:shadow-md dark:border-white/[0.06] dark:bg-[#161b22] dark:hover:border-white/15"
        >
          <div class="flex items-center gap-3">
            <div class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl bg-[#e8f2ff] text-[#0355cd] dark:bg-[#0355cd]/25 dark:text-[#60a5fa]">
              <Icon name="cube" size="sm" />
            </div>
            <div class="min-w-0">
              <p class="truncate font-mono text-xs font-semibold text-gray-900 dark:text-white" :title="log.model">
                {{ log.model }}
              </p>
              <p class="mt-0.5 text-[11px] text-gray-400 dark:text-gray-500">
                {{ formatDateTime(log.created_at) }}
              </p>
            </div>
          </div>

          <div class="text-right">
            <div class="flex items-baseline justify-end gap-1.5">
              <span class="font-mono text-xs font-bold text-emerald-600 dark:text-emerald-400" :title="t('dashboard.actual')">
                ${{ formatCost(log.actual_cost) }}
              </span>
              <span class="font-mono text-[11px] text-gray-400 dark:text-gray-500" :title="t('dashboard.standard')">
                / ${{ formatCost(log.total_cost) }}
              </span>
            </div>
            <span class="bap-pill bap-pill-gray mt-1 text-[10px]">
              {{ (log.input_tokens + log.output_tokens).toLocaleString() }} tokens
            </span>
          </div>
        </div>

        <router-link
          to="/usage"
          class="group mt-3 flex items-center justify-center gap-1.5 rounded-xl border border-black/[0.05] bg-black/[0.02] py-2.5 text-xs font-semibold text-[#4791ff] transition-all hover:bg-[#e8f2ff]/50 hover:text-[#0355cd] dark:border-white/[0.06] dark:bg-white/[0.03] dark:hover:bg-[#0355cd]/15 dark:hover:text-[#60a5fa]"
        >
          <span>{{ t('dashboard.viewAllUsage') }}</span>
          <Icon name="arrowRight" size="xs" class="transition-transform group-hover:translate-x-1" />
        </router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'
import type { UsageLog } from '@/types'

defineProps<{
  data: UsageLog[]
  loading: boolean
}>()
const { t } = useI18n()
const formatCost = (c: number) => c.toFixed(4)
</script>
