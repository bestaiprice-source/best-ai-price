<template>
  <div class="space-y-6">
    <!-- Date Range & Granularity Filter Toolbar -->
    <div class="bap-card p-3 sm:p-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <!-- Date Range -->
        <div class="flex items-center gap-2.5">
          <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-black/[0.04] text-gray-600 dark:bg-white/[0.06] dark:text-gray-300">
            <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
            </svg>
          </div>
          <span class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
            {{ t('dashboard.timeRange') }}:
          </span>
          <DateRangePicker
            :start-date="startDate"
            :end-date="endDate"
            @update:startDate="$emit('update:startDate', $event)"
            @update:endDate="$emit('update:endDate', $event)"
            @change="$emit('dateRangeChange', $event)"
          />
        </div>

        <!-- Granularity Mode Switcher (bestaiprice segmented pill style) -->
        <div class="flex items-center gap-2">
          <span class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
            {{ t('dashboard.granularity') }}:
          </span>
          <div class="inline-flex rounded-xl border border-black/[0.06] bg-black/[0.03] p-1 dark:border-white/[0.08] dark:bg-white/[0.04]">
            <button
              type="button"
              class="rounded-lg px-3 py-1 text-xs font-medium transition-all"
              :class="granularity === 'day'
                ? 'bg-[#4791ff] text-white shadow-sm'
                : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
              @click="setGranularity('day')"
            >
              {{ t('dashboard.day') }}
            </button>
            <button
              type="button"
              class="rounded-lg px-3 py-1 text-xs font-medium transition-all"
              :class="granularity === 'hour'
                ? 'bg-[#4791ff] text-white shadow-sm'
                : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
              @click="setGranularity('hour')"
            >
              {{ t('dashboard.hour') }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Charts Grid -->
    <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <!-- Model Distribution Chart -->
      <div class="bap-card relative overflow-hidden p-5">
        <div v-if="loading" class="absolute inset-0 z-10 flex items-center justify-center bg-white/70 dark:bg-dark-900/70">
          <LoadingSpinner size="md" />
        </div>

        <div class="mb-4 flex items-center justify-between border-b border-black/[0.04] pb-3 dark:border-white/[0.06]">
          <div class="flex items-center gap-2">
            <h3 class="text-sm font-semibold tracking-tight text-gray-900 dark:text-white">
              {{ t('dashboard.modelDistribution') }}
            </h3>
            <span v-if="models?.length" class="bap-pill bap-pill-blue text-[10px]">
              {{ models.length }} models
            </span>
          </div>
          <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('ui.userDashboardCharts.byTokenVolume') }}</span>
        </div>

        <div class="flex flex-col items-center gap-6 sm:flex-row">
          <div class="h-44 w-44 flex-shrink-0">
            <Doughnut v-if="modelData" :data="modelData" :options="doughnutOptions" />
            <div v-else class="flex h-full items-center justify-center text-xs text-gray-400 dark:text-gray-500">
              {{ t('dashboard.noDataAvailable') }}
            </div>
          </div>

          <div class="max-h-48 w-full flex-1 overflow-y-auto pr-1">
            <table class="w-full text-xs">
              <thead>
                <tr class="border-b border-black/[0.04] text-[10px] uppercase tracking-wider text-gray-400 dark:border-white/[0.06] dark:text-gray-500">
                  <th class="pb-2 text-left font-medium">{{ t('dashboard.model') }}</th>
                  <th class="pb-2 text-right font-medium">{{ t('dashboard.requests') }}</th>
                  <th class="pb-2 text-right font-medium">{{ t('dashboard.tokens') }}</th>
                  <th class="pb-2 text-right font-medium">{{ t('dashboard.actual') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-black/[0.03] dark:divide-white/[0.04]">
                <tr
                  v-for="model in models"
                  :key="model.model"
                  class="transition-colors hover:bg-black/[0.02] dark:hover:bg-white/[0.02]"
                >
                  <td class="max-w-[120px] truncate py-2 font-medium" :title="model.model">
                    <span class="rounded bg-black/[0.04] px-1.5 py-0.5 font-mono text-[11px] text-gray-800 dark:bg-white/[0.06] dark:text-gray-200">
                      {{ model.model }}
                    </span>
                  </td>
                  <td class="py-2 text-right font-mono text-gray-600 dark:text-gray-400">
                    {{ formatNumber(model.requests) }}
                  </td>
                  <td class="py-2 text-right font-mono font-medium text-gray-900 dark:text-white">
                    {{ formatTokens(model.total_tokens) }}
                  </td>
                  <td class="py-2 text-right font-mono font-semibold text-emerald-600 dark:text-emerald-400">
                    ${{ formatCost(model.actual_cost) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <!-- Token Usage Trend Chart -->
      <TokenUsageTrend :trend-data="trend" :loading="loading" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import { Doughnut } from 'vue-chartjs'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import type { TrendDataPoint, ModelStat } from '@/types'
import {
  formatCostFixed as formatCost,
  formatNumberLocaleString as formatNumber,
  formatTokensK as formatTokens
} from '@/utils/format'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  ArcElement,
  Title,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  ArcElement,
  Title,
  Tooltip,
  Legend,
  Filler
)

const props = defineProps<{
  loading: boolean
  startDate: string
  endDate: string
  granularity: string
  trend: TrendDataPoint[]
  models: ModelStat[]
}>()

const emit = defineEmits([
  'update:startDate',
  'update:endDate',
  'update:granularity',
  'dateRangeChange',
  'granularityChange'
])
const { t } = useI18n()

const setGranularity = (val: string) => {
  emit('update:granularity', val)
  emit('granularityChange')
}

const modelData = computed(() => {
  if (!props.models?.length) return null
  return {
    labels: props.models.map((m: ModelStat) => m.model),
    datasets: [
      {
        data: props.models.map((m: ModelStat) => m.total_tokens),
        backgroundColor: [
          '#4791ff',
          '#10b981',
          '#8b5cf6',
          '#f59e0b',
          '#06b6d4',
          '#ec4899',
          '#3b82f6',
          '#14b8a6'
        ],
        borderWidth: 0
      }
    ]
  }
})

const doughnutOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: false },
    tooltip: {
      callbacks: {
        label: (context: any) => `${context.label}: ${formatTokens(context.parsed)} tokens`
      }
    }
  },
  cutout: '72%'
}
</script>
