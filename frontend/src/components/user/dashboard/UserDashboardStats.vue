<template>
  <!-- Row 1: Core Financial & Activity Stats -->
  <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
    <!-- Balance -->
    <div v-if="!isSimple" class="bap-stat-card group">
      <div class="flex items-start justify-between">
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-emerald-50 text-emerald-600 transition-transform group-hover:scale-105 dark:bg-emerald-950/40 dark:text-emerald-400">
            <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v12m-3-2.818l.879.659c1.171.879 3.07.879 4.242 0 1.172-.879 1.172-2.303 0-3.182C13.536 12.219 12.768 12 12 12c-.725 0-1.45-.22-2.003-.659-1.106-.879-1.106-2.303 0-3.182s2.9-.879 4.006 0l.415.33M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
          </div>
          <div>
            <p class="text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
              {{ t('dashboard.balance') }}
            </p>
            <p class="mt-0.5 font-mono text-2xl font-bold tracking-tight text-emerald-600 dark:text-emerald-400">
              ${{ formatBalance(balance) }}
            </p>
          </div>
        </div>
        <router-link
          to="/subscriptions"
          class="bap-pill bap-pill-green text-[11px] font-medium transition-colors hover:bg-emerald-200 dark:hover:bg-emerald-900/60"
        >
          {{ t('common.available') }}
        </router-link>
      </div>
      <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
        <span>{{ t('dashboard.todayCost') }}: <span class="font-mono font-medium text-gray-900 dark:text-white">${{ formatCost(stats?.today_actual_cost || 0) }}</span></span>
      </div>
    </div>

    <!-- Active API Keys -->
    <div class="bap-stat-card group">
      <div class="flex items-start justify-between">
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-blue-50 text-[#0355cd] transition-transform group-hover:scale-105 dark:bg-[#0355cd]/20 dark:text-[#60a5fa]">
            <Icon name="key" size="md" :stroke-width="2" />
          </div>
          <div>
            <p class="text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
              {{ t('dashboard.apiKeys') }}
            </p>
            <p class="mt-0.5 font-mono text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
              {{ stats?.total_api_keys || 0 }}
            </p>
          </div>
        </div>
        <span class="bap-pill bap-pill-blue">
          {{ stats?.active_api_keys || 0 }} {{ t('common.active') }}
        </span>
      </div>
      <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
        <span>{{ t('common.total') }}: {{ stats?.total_api_keys || 0 }}</span>
        <router-link to="/keys" class="font-medium text-[#4791ff] hover:underline">
          {{ t('dashboard.createApiKey') }} &rarr;
        </router-link>
      </div>
    </div>

    <!-- Today Requests -->
    <div class="bap-stat-card group">
      <div class="flex items-start justify-between">
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-indigo-50 text-indigo-600 transition-transform group-hover:scale-105 dark:bg-indigo-950/40 dark:text-indigo-400">
            <Icon name="chart" size="md" :stroke-width="2" />
          </div>
          <div>
            <p class="text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
              {{ t('dashboard.todayRequests') }}
            </p>
            <p class="mt-0.5 font-mono text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
              {{ formatNumber(stats?.today_requests || 0) }}
            </p>
          </div>
        </div>
        <span class="bap-pill bap-pill-gray">
          {{ t('dashboard.day') }}
        </span>
      </div>
      <!-- Sparkline + "vs yesterday", from the 7-day trend the dashboard already loads. -->
      <div v-if="requestsSpark.length > 1" class="mt-3 flex items-center justify-between gap-3">
        <SparkLine :values="requestsSpark" class="h-[24px] w-full text-[#4791ff]" />
        <StatDelta :delta="requestsDelta" :format="formatNumber" />
      </div>
      <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
        <span>{{ t('common.total') }}: <span class="font-mono font-medium text-gray-900 dark:text-white">{{ formatNumber(stats?.total_requests || 0) }}</span></span>
        <router-link to="/usage" class="font-medium text-[#4791ff] hover:underline">
          {{ t('dashboard.viewUsage') }} &rarr;
        </router-link>
      </div>
    </div>

    <!-- Cost Overview -->
    <div class="bap-stat-card group">
      <div class="flex items-start justify-between">
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-purple-50 text-purple-600 transition-transform group-hover:scale-105 dark:bg-purple-950/40 dark:text-purple-400">
            <Icon name="dollar" size="md" :stroke-width="2" />
          </div>
          <div>
            <p class="text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
              {{ t('dashboard.todayCost') }}
            </p>
            <div class="mt-0.5 flex items-baseline gap-1">
              <span class="font-mono text-2xl font-bold tracking-tight text-purple-600 dark:text-purple-400">
                ${{ formatCost(stats?.today_actual_cost || 0) }}
              </span>
              <span class="font-mono text-xs text-gray-400 dark:text-gray-500" :title="t('dashboard.standard')">
                / ${{ formatCost(stats?.today_cost || 0) }}
              </span>
            </div>
          </div>
        </div>
        <span class="bap-pill bap-pill-purple">
          {{ t('dashboard.actual') }}
        </span>
      </div>
      <!-- Rising spend is not "good", hence the inverse colour polarity. -->
      <div v-if="costSpark.length > 1" class="mt-3 flex items-center justify-between gap-3">
        <SparkLine :values="costSpark" class="h-[24px] w-full text-purple-500" />
        <StatDelta :delta="costDelta" :format="formatCost" inverse />
      </div>
      <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
        <span>{{ t('common.total') }}: <span class="font-mono font-medium text-purple-600 dark:text-purple-400">${{ formatCost(stats?.total_actual_cost || 0) }}</span></span>
        <span class="font-mono text-gray-400 dark:text-gray-500">${{ formatCost(stats?.total_cost || 0) }}</span>
      </div>
    </div>
  </div>

  <!-- Row 2: Token Processing & Performance -->
  <div class="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
    <!-- Today Tokens -->
    <div class="bap-stat-card group">
      <div class="flex items-center gap-3">
        <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-amber-50 text-amber-600 transition-transform group-hover:scale-105 dark:bg-amber-950/40 dark:text-amber-400">
          <Icon name="cube" size="md" :stroke-width="2" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
            {{ t('dashboard.todayTokens') }}
          </p>
          <p class="mt-0.5 font-mono text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
            {{ formatTokens(stats?.today_tokens || 0) }}
          </p>
        </div>
      </div>
      <div v-if="tokensSpark.length > 1" class="mt-3 flex items-center justify-between gap-3">
        <SparkLine :values="tokensSpark" class="h-[24px] w-full text-amber-500" />
        <StatDelta :delta="tokensDelta" :format="formatTokens" />
      </div>
      <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 font-mono text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
        <span>In: <strong class="text-gray-900 dark:text-white">{{ formatTokens(stats?.today_input_tokens || 0) }}</strong></span>
        <span>{{ t('ui.userDashboardStats.out') }} <strong class="text-gray-900 dark:text-white">{{ formatTokens(stats?.today_output_tokens || 0) }}</strong></span>
      </div>
    </div>

    <!-- Total Tokens -->
    <div class="bap-stat-card group">
      <div class="flex items-center gap-3">
        <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-sky-50 text-sky-600 transition-transform group-hover:scale-105 dark:bg-sky-950/40 dark:text-sky-400">
          <Icon name="database" size="md" :stroke-width="2" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
            {{ t('dashboard.totalTokens') }}
          </p>
          <p class="mt-0.5 font-mono text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
            {{ formatTokens(stats?.total_tokens || 0) }}
          </p>
        </div>
      </div>
      <!-- Cumulative curve: a running total has no meaningful day-over-day delta. -->
      <div v-if="cumulativeTokensSpark.length > 1" class="mt-3 flex items-center">
        <SparkLine :values="cumulativeTokensSpark" class="h-[24px] w-full text-sky-500" />
      </div>
      <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 font-mono text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
        <span>In: <strong class="text-gray-900 dark:text-white">{{ formatTokens(stats?.total_input_tokens || 0) }}</strong></span>
        <span>{{ t('ui.userDashboardStats.out2') }} <strong class="text-gray-900 dark:text-white">{{ formatTokens(stats?.total_output_tokens || 0) }}</strong></span>
      </div>
    </div>

    <!-- Performance (RPM/TPM) -->
    <div class="bap-stat-card group">
      <div class="flex items-center gap-3">
        <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-violet-50 text-violet-600 transition-transform group-hover:scale-105 dark:bg-violet-950/40 dark:text-violet-400">
          <Icon name="bolt" size="md" :stroke-width="2" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
            {{ t('dashboard.performance') }}
          </p>
          <div class="mt-0.5 flex items-baseline gap-2">
            <span class="font-mono text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
              {{ formatTokens(stats?.rpm || 0) }}
            </span>
            <span class="bap-pill bap-pill-purple text-[10px] uppercase">RPM</span>
          </div>
        </div>
      </div>
      <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
        <span>{{ t('ui.userDashboardStats.throughput') }}</span>
        <span class="font-mono font-semibold text-violet-600 dark:text-violet-400">
          {{ formatTokens(stats?.tpm || 0) }} TPM
        </span>
      </div>
    </div>

    <!-- Avg Response Time -->
    <div class="bap-stat-card group">
      <div class="flex items-center gap-3">
        <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-rose-50 text-rose-600 transition-transform group-hover:scale-105 dark:bg-rose-950/40 dark:text-rose-400">
          <Icon name="clock" size="md" :stroke-width="2" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
            {{ t('dashboard.avgResponse') }}
          </p>
          <p class="mt-0.5 font-mono text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
            {{ formatDuration(stats?.average_duration_ms || 0) }}
          </p>
        </div>
      </div>
      <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
        <span class="flex items-center gap-1.5">
          <span class="h-2 w-2 rounded-full bg-emerald-500"></span>
          <span>{{ t('ui.userDashboardStats.latency') }}</span>
        </span>
        <span class="font-medium text-emerald-600 dark:text-emerald-400">{{ t('ui.userDashboardStats.optimal') }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import SparkLine from './SparkLine.vue'
import StatDelta from './StatDelta.vue'
import { cumulativeSeries, deltaVsPreviousDay, metricSeries } from '@/utils/dashboardTrend'
import type { UserDashboardStats as UserStatsType } from '@/api/usage'
import type { TrendDataPoint } from '@/types'

const props = defineProps<{
  stats: UserStatsType
  balance: number
  isSimple: boolean
  /**
   * Daily usage trend already fetched by the dashboard. Only the metrics the
   * trend aggregates per day get a sparkline; cards without a series stay plain.
   */
  trend?: TrendDataPoint[]
}>()
const { t } = useI18n()

const formatBalance = (b: number) =>
  new Intl.NumberFormat('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(b)

const formatNumber = (n: number) => n.toLocaleString()
const formatCost = (c: number) => c.toFixed(4)
const formatTokens = (t: number) => {
  if (t >= 1_000_000) return `${(t / 1_000_000).toFixed(1)}M`
  if (t >= 1000) return `${(t / 1000).toFixed(1)}K`
  return t.toString()
}
const formatDuration = (ms: number) => (ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${ms.toFixed(0)}ms`)

// Sparkline series + "vs yesterday" deltas, derived from the loaded trend.
const requestsSpark = computed(() => metricSeries(props.trend, 'requests'))
const requestsDelta = computed(() => deltaVsPreviousDay(props.trend, 'requests'))
const costSpark = computed(() => metricSeries(props.trend, 'actual_cost'))
const costDelta = computed(() => deltaVsPreviousDay(props.trend, 'actual_cost'))
const tokensSpark = computed(() => metricSeries(props.trend, 'total_tokens'))
const tokensDelta = computed(() => deltaVsPreviousDay(props.trend, 'total_tokens'))
const cumulativeTokensSpark = computed(() => cumulativeSeries(metricSeries(props.trend, 'total_tokens')))
</script>
