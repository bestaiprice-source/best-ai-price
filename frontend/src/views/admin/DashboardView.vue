<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- BestAiPrice Styled Admin Hero Banner -->
      <div class="bap-card relative overflow-hidden p-6 sm:p-7">
        <div class="flex flex-col justify-between gap-4 md:flex-row md:items-center">
          <div class="space-y-2">
            <div class="flex flex-wrap items-center gap-2.5">
              <span class="inline-flex items-center gap-1.5 rounded-full bg-emerald-50 px-2.5 py-0.5 text-xs font-semibold text-emerald-600 dark:bg-emerald-950/40 dark:text-emerald-400">
                <span class="relative flex h-2 w-2">
                  <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75"></span>
                  <span class="relative inline-flex h-2 w-2 rounded-full bg-emerald-500"></span>
                </span>
                {{ t('ui.dashboardView.hypervisorActive') }}
              </span>
              <span class="bap-pill bap-pill-purple uppercase text-[10px]">
                {{ t('ui.dashboardView.adminConsole') }}
              </span>
            </div>
            <h1 class="text-xl font-bold tracking-tight text-gray-900 sm:text-2xl dark:text-white">
              {{ t('admin.dashboard.title') }}
            </h1>
            <p class="text-xs text-gray-500 sm:text-sm dark:text-gray-400">
              System-wide metrics, upstream accounts health, active proxy clusters, and token usage trends.
            </p>
          </div>

          <!-- Shortcuts -->
          <div class="flex flex-wrap items-center gap-2.5">
            <router-link
              to="/admin/accounts"
              class="bap-btn-accent"
            >
              <Icon name="server" size="xs" />
              <span>{{ t('admin.dashboard.accounts') }}</span>
            </router-link>
            <router-link
              to="/admin/users"
              class="inline-flex items-center gap-1.5 rounded-xl border border-black/[0.08] bg-white px-3.5 py-2 text-xs font-semibold text-gray-700 shadow-sm transition-all hover:bg-gray-50 dark:border-white/[0.08] dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
            >
              <Icon name="users" size="xs" />
              <span>{{ t('admin.dashboard.users') }}</span>
            </router-link>
          </div>
        </div>
      </div>

      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-16">
        <LoadingSpinner size="lg" />
      </div>

      <template v-else-if="stats">
        <!-- Row 1: Core System Stats -->
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <!-- Total API Keys -->
          <div class="bap-stat-card group">
            <div class="flex items-start justify-between">
              <div class="flex items-center gap-3">
                <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-blue-50 text-[#0355cd] transition-transform group-hover:scale-105 dark:bg-[#0355cd]/20 dark:text-[#60a5fa]">
                  <Icon name="key" size="md" :stroke-width="2" />
                </div>
                <div>
                  <p class="text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.dashboard.apiKeys') }}
                  </p>
                  <p class="mt-0.5 font-mono text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                    {{ stats.total_api_keys }}
                  </p>
                </div>
              </div>
              <span class="bap-pill bap-pill-blue">
                {{ stats.active_api_keys }} {{ t('common.active') }}
              </span>
            </div>
            <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
              <span>{{ t('ui.dashboardView.status') }} <strong class="text-emerald-600 dark:text-emerald-400">{{ stats.active_api_keys }} Online</strong></span>
              <router-link to="/keys" class="font-medium text-[#4791ff] hover:underline">
                {{ t('ui.dashboardView.manage') }}
              </router-link>
            </div>
          </div>

          <!-- Service Accounts -->
          <div class="bap-stat-card group">
            <div class="flex items-start justify-between">
              <div class="flex items-center gap-3">
                <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-purple-50 text-purple-600 transition-transform group-hover:scale-105 dark:bg-purple-950/40 dark:text-purple-400">
                  <Icon name="server" size="md" :stroke-width="2" />
                </div>
                <div>
                  <p class="text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.dashboard.accounts') }}
                  </p>
                  <p class="mt-0.5 font-mono text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                    {{ stats.total_accounts }}
                  </p>
                </div>
              </div>
              <span class="bap-pill" :class="stats.error_accounts > 0 ? 'bap-pill-red' : 'bap-pill-green'">
                {{ stats.normal_accounts }} {{ t('common.active') }}
              </span>
            </div>
            <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
              <span v-if="stats.error_accounts > 0" class="font-medium text-rose-500">
                {{ stats.error_accounts }} {{ t('common.error') }}
              </span>
              <span v-else class="text-emerald-600 dark:text-emerald-400">{{ t('ui.dashboardView.allNodesNormal') }}</span>
              <router-link to="/admin/accounts" class="font-medium text-[#4791ff] hover:underline">
                {{ t('ui.dashboardView.view') }}
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
                    {{ t('admin.dashboard.todayRequests') }}
                  </p>
                  <p class="mt-0.5 font-mono text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                    {{ formatNumber(stats.today_requests) }}
                  </p>
                </div>
              </div>
              <span class="bap-pill bap-pill-gray">
                {{ t('dashboard.day') }}
              </span>
            </div>
            <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
              <span>{{ t('common.total') }}: <span class="font-mono font-medium text-gray-900 dark:text-white">{{ formatNumber(stats.total_requests) }}</span></span>
              <router-link to="/admin/usage" class="font-medium text-[#4791ff] hover:underline">
                {{ t('ui.dashboardView.logs') }}
              </router-link>
            </div>
          </div>

          <!-- New Users Today -->
          <div class="bap-stat-card group">
            <div class="flex items-start justify-between">
              <div class="flex items-center gap-3">
                <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-emerald-50 text-emerald-600 transition-transform group-hover:scale-105 dark:bg-emerald-950/40 dark:text-emerald-400">
                  <Icon name="userPlus" size="md" :stroke-width="2" />
                </div>
                <div>
                  <p class="text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                    {{ t('admin.dashboard.users') }}
                  </p>
                  <p class="mt-0.5 font-mono text-2xl font-bold tracking-tight text-emerald-600 dark:text-emerald-400">
                    +{{ stats.today_new_users }}
                  </p>
                </div>
              </div>
              <span class="bap-pill bap-pill-green">
                {{ t('ui.dashboardView.newToday') }}
              </span>
            </div>
            <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
              <span>{{ t('common.total') }}: <span class="font-mono font-medium text-gray-900 dark:text-white">{{ formatNumber(stats.total_users) }}</span></span>
              <router-link to="/admin/users" class="font-medium text-[#4791ff] hover:underline">
                {{ t('ui.dashboardView.users') }}
              </router-link>
            </div>
          </div>
        </div>

        <!-- Row 2: Token Processing & System Performance -->
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <!-- Today Tokens -->
          <div class="bap-stat-card group">
            <div class="flex items-center gap-3">
              <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-amber-50 text-amber-600 transition-transform group-hover:scale-105 dark:bg-amber-950/40 dark:text-amber-400">
                <Icon name="cube" size="md" :stroke-width="2" />
              </div>
              <div class="min-w-0 flex-1">
                <p class="text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.todayTokens') }}
                </p>
                <p class="mt-0.5 font-mono text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                  {{ formatTokens(stats.today_tokens) }}
                </p>
              </div>
            </div>
            <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 font-mono text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
              <span>{{ t('ui.dashboardView.cost') }} <strong class="text-amber-600 dark:text-amber-400">${{ formatCost(stats.today_actual_cost) }}</strong></span>
              <span class="text-gray-400 dark:text-gray-500">${{ formatCost(stats.today_cost) }}</span>
            </div>
          </div>

          <!-- Total Tokens -->
          <div class="bap-stat-card group">
            <div class="flex items-center gap-3">
              <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-indigo-50 text-indigo-600 transition-transform group-hover:scale-105 dark:bg-indigo-950/40 dark:text-indigo-400">
                <Icon name="database" size="md" :stroke-width="2" />
              </div>
              <div class="min-w-0 flex-1">
                <p class="text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.totalTokens') }}
                </p>
                <p class="mt-0.5 font-mono text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                  {{ formatTokens(stats.total_tokens) }}
                </p>
              </div>
            </div>
            <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 font-mono text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
              <span>{{ t('ui.dashboardView.cost2') }} <strong class="text-indigo-600 dark:text-indigo-400">${{ formatCost(stats.total_actual_cost) }}</strong></span>
              <span class="text-gray-400 dark:text-gray-500">${{ formatCost(stats.total_cost) }}</span>
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
                  {{ t('admin.dashboard.performance') }}
                </p>
                <div class="mt-0.5 flex items-baseline gap-2">
                  <span class="font-mono text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                    {{ formatTokens(stats.rpm) }}
                  </span>
                  <span class="bap-pill bap-pill-purple text-[10px] uppercase">RPM</span>
                </div>
              </div>
            </div>
            <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
              <span>{{ t('ui.dashboardView.throughput') }}</span>
              <span class="font-mono font-semibold text-violet-600 dark:text-violet-400">
                {{ formatTokens(stats.tpm) }} TPM
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
                  {{ t('admin.dashboard.avgResponse') }}
                </p>
                <p class="mt-0.5 font-mono text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
                  {{ formatDuration(stats.average_duration_ms) }}
                </p>
              </div>
            </div>
            <div class="mt-4 flex items-center justify-between border-t border-black/[0.04] pt-3 text-xs text-gray-500 dark:border-white/[0.06] dark:text-gray-400">
              <span>{{ t('ui.dashboardView.activeusers') }} <strong class="text-gray-900 dark:text-white">{{ stats.active_users }}</strong></span>
              <span class="font-medium text-emerald-600 dark:text-emerald-400">{{ t('ui.dashboardView.live') }}</span>
            </div>
          </div>
        </div>

        <!-- Hypervisor Monitoring & Node Control Section -->
        <HypervisorDashboardSection />

        <!-- Charts Section -->
        <div class="space-y-6">
          <!-- Date Range Filter Toolbar (bestaiprice segmented pill style) -->
          <div class="bap-card p-3.5 sm:p-4">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div class="flex items-center gap-2.5">
                <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-black/[0.04] text-gray-600 dark:bg-white/[0.06] dark:text-gray-300">
                  <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
                  </svg>
                </div>
                <span class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.timeRange') }}:
                </span>
                <DateRangePicker
                  v-model:start-date="startDate"
                  v-model:end-date="endDate"
                  @change="onDateRangeChange"
                />
              </div>

              <div class="flex items-center gap-2">
                <span class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.granularity') }}:
                </span>
                <div class="inline-flex rounded-xl border border-black/[0.06] bg-black/[0.03] p-1 dark:border-white/[0.08] dark:bg-white/[0.04]">
                  <button
                    type="button"
                    class="rounded-lg px-3 py-1 text-xs font-medium transition-all"
                    :class="granularity === 'day'
                      ? 'bg-[#4791ff] text-white shadow-sm'
                      : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
                    @click="granularity = 'day'; loadChartData()"
                  >
                    {{ t('admin.dashboard.day') }}
                  </button>
                  <button
                    type="button"
                    class="rounded-lg px-3 py-1 text-xs font-medium transition-all"
                    :class="granularity === 'hour'
                      ? 'bg-[#4791ff] text-white shadow-sm'
                      : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
                    @click="granularity = 'hour'; loadChartData()"
                  >
                    {{ t('admin.dashboard.hour') }}
                  </button>
                </div>
              </div>
            </div>
          </div>

          <!-- Charts Grid -->
          <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <ModelDistributionChart
              :model-stats="modelStats"
              :enable-ranking-view="true"
              :ranking-items="rankingItems"
              :ranking-total-actual-cost="rankingTotalActualCost"
              :ranking-total-requests="rankingTotalRequests"
              :ranking-total-tokens="rankingTotalTokens"
              :loading="chartsLoading"
              :ranking-loading="rankingLoading"
              :ranking-error="rankingError"
              :start-date="startDate"
              :end-date="endDate"
              @ranking-click="goToUserUsage"
            />
            <TokenUsageTrend :trend-data="trendData" :loading="chartsLoading" />
          </div>

          <!-- User Usage Trend (Full Width) -->
          <div class="bap-card p-5">
            <div class="mb-4 flex items-center justify-between border-b border-black/[0.04] pb-3 dark:border-white/[0.06]">
              <div class="flex items-center gap-2">
                <h3 class="text-sm font-semibold tracking-tight text-gray-900 dark:text-white">
                  {{ t('admin.dashboard.recentUsage') }}
                </h3>
                <span class="bap-pill bap-pill-blue text-[10px]">{{ t('ui.dashboardView.top12Users') }}</span>
              </div>
              <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('ui.dashboardView.tokenConsumption') }}</span>
            </div>
            <div class="h-64">
              <div v-if="userTrendLoading" class="flex h-full items-center justify-center">
                <LoadingSpinner size="md" />
              </div>
              <Line v-else-if="userTrendChartData" :data="userTrendChartData" :options="lineOptions" />
              <div
                v-else
                class="flex h-full items-center justify-center text-xs text-gray-400 dark:text-gray-500"
              >
                {{ t('admin.dashboard.noDataAvailable') }}
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
import { adminAPI } from '@/api/admin'
import type {
  DashboardStats,
  TrendDataPoint,
  ModelStat,
  UserUsageTrendPoint,
  UserSpendingRankingItem
} from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import HypervisorDashboardSection from './components/HypervisorDashboardSection.vue'

import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'

// Register Chart.js components
ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler
)

const appStore = useAppStore()
const router = useRouter()
const stats = ref<DashboardStats | null>(null)
const loading = ref(false)
const chartsLoading = ref(false)
const userTrendLoading = ref(false)
const rankingLoading = ref(false)
const rankingError = ref(false)

// Chart data
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const userTrend = ref<UserUsageTrendPoint[]>([])
const rankingItems = ref<UserSpendingRankingItem[]>([])
const rankingTotalActualCost = ref(0)
const rankingTotalRequests = ref(0)
const rankingTotalTokens = ref(0)
let chartLoadSeq = 0
let usersTrendLoadSeq = 0
let rankingLoadSeq = 0
const rankingLimit = 12

// Helper function to format date in local timezone
const formatLocalDate = (date: Date): string => {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

const getLast24HoursRangeDates = (): { start: string; end: string } => {
  const end = new Date()
  const start = new Date(end.getTime() - 24 * 60 * 60 * 1000)
  return {
    start: formatLocalDate(start),
    end: formatLocalDate(end)
  }
}

// Date range
const granularity = ref<'day' | 'hour'>('hour')
const defaultRange = getLast24HoursRangeDates()
const startDate = ref(defaultRange.start)
const endDate = ref(defaultRange.end)

// Dark mode detection
const isDarkMode = computed(() => {
  return document.documentElement.classList.contains('dark')
})

// Chart colors
const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb'
}))

// Line chart options (for user trend chart)
const lineOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    intersect: false,
    mode: 'index' as const
  },
  plugins: {
    legend: {
      position: 'top' as const,
      labels: {
        color: chartColors.value.text,
        usePointStyle: true,
        pointStyle: 'circle',
        padding: 15,
        font: {
          size: 11
        }
      }
    },
    tooltip: {
      itemSort: (a: any, b: any) => {
        const aValue = typeof a?.raw === 'number' ? a.raw : Number(a?.parsed?.y ?? 0)
        const bValue = typeof b?.raw === 'number' ? b.raw : Number(b?.parsed?.y ?? 0)
        return bValue - aValue
      },
      callbacks: {
        label: (context: any) => {
          return `${context.dataset.label}: ${formatTokens(context.raw)}`
        }
      }
    }
  },
  scales: {
    x: {
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        }
      }
    },
    y: {
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        },
        callback: (value: string | number) => formatTokens(Number(value))
      }
    }
  }
}))

// User trend chart data
const userTrendChartData = computed(() => {
  if (!userTrend.value?.length) return null

  const getDisplayName = (point: UserUsageTrendPoint): string => {
    const username = point.username?.trim()
    if (username) {
      return username
    }

    const email = point.email?.trim()
    if (email) {
      return email
    }

    return t('admin.redeem.userPrefix', { id: point.user_id })
  }

  // Group by user_id to avoid merging different users with the same display name
  const userGroups = new Map<number, { name: string; data: Map<string, number> }>()
  const allDates = new Set<string>()

  userTrend.value.forEach((point) => {
    allDates.add(point.date)
    const key = point.user_id
    if (!userGroups.has(key)) {
      userGroups.set(key, { name: getDisplayName(point), data: new Map() })
    }
    userGroups.get(key)!.data.set(point.date, point.tokens)
  })

  const sortedDates = Array.from(allDates).sort()
  const colors = [
    '#3b82f6',
    '#10b981',
    '#f59e0b',
    '#ef4444',
    '#8b5cf6',
    '#ec4899',
    '#14b8a6',
    '#f97316',
    '#6366f1',
    '#84cc16',
    '#06b6d4',
    '#a855f7'
  ]

  const datasets = Array.from(userGroups.values()).map((group, idx) => ({
    label: group.name,
    data: sortedDates.map((date) => group.data.get(date) || 0),
    borderColor: colors[idx % colors.length],
    backgroundColor: `${colors[idx % colors.length]}20`,
    fill: false,
    tension: 0.3
  }))

  return {
    labels: sortedDates,
    datasets
  }
})

// Format helpers
const formatTokens = (value: number | undefined): string => {
  if (value === undefined || value === null) return '0'
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)}B`
  } else if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)}M`
  } else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)}K`
  }
  return value.toLocaleString()
}

const formatNumber = (value: number): string => {
  return value.toLocaleString()
}

const formatCost = (value: number): string => {
  if (value >= 1000) {
    return (value / 1000).toFixed(2) + 'K'
  } else if (value >= 1) {
    return value.toFixed(2)
  } else if (value >= 0.01) {
    return value.toFixed(3)
  }
  return value.toFixed(4)
}

const formatDuration = (ms: number): string => {
  if (ms >= 1000) {
    return `${(ms / 1000).toFixed(2)}s`
  }
  return `${Math.round(ms)}ms`
}

const goToUserUsage = (item: UserSpendingRankingItem) => {
  void router.push({
    path: '/admin/usage',
    query: {
      user_id: String(item.user_id),
      start_date: startDate.value,
      end_date: endDate.value
    }
  })
}

// Date range change handler
const onDateRangeChange = (range: {
  startDate: string
  endDate: string
  preset: string | null
}) => {
  // Auto-select granularity based on date range
  const start = new Date(range.startDate)
  const end = new Date(range.endDate)
  const daysDiff = Math.ceil((end.getTime() - start.getTime()) / (1000 * 60 * 60 * 24))

  // If range is 1 day, use hourly granularity
  if (daysDiff <= 1) {
    granularity.value = 'hour'
  } else {
    granularity.value = 'day'
  }

  loadChartData()
}

// Load data
const loadDashboardSnapshot = async (includeStats: boolean) => {
  const currentSeq = ++chartLoadSeq
  if (includeStats && !stats.value) {
    loading.value = true
  }
  chartsLoading.value = true
  try {
    const response = await adminAPI.dashboard.getSnapshotV2({
      start_date: startDate.value,
      end_date: endDate.value,
      granularity: granularity.value,
      include_stats: includeStats,
      include_trend: true,
      include_model_stats: true,
      include_group_stats: false,
      include_users_trend: false
    })
    if (currentSeq !== chartLoadSeq) return
    if (includeStats && response.stats) {
      stats.value = response.stats
    }
    trendData.value = response.trend || []
    modelStats.value = response.models || []
  } catch (error) {
    if (currentSeq !== chartLoadSeq) return
    appStore.showError(t('admin.dashboard.failedToLoad'))
    console.error('Error loading dashboard snapshot:', error)
  } finally {
    if (currentSeq === chartLoadSeq) {
      loading.value = false
      chartsLoading.value = false
    }
  }
}

const loadUsersTrend = async () => {
  const currentSeq = ++usersTrendLoadSeq
  userTrendLoading.value = true
  try {
    const response = await adminAPI.dashboard.getUserUsageTrend({
      start_date: startDate.value,
      end_date: endDate.value,
      granularity: granularity.value,
      limit: 12
    })
    if (currentSeq !== usersTrendLoadSeq) return
    userTrend.value = response.trend || []
  } catch (error) {
    if (currentSeq !== usersTrendLoadSeq) return
    console.error('Error loading users trend:', error)
    userTrend.value = []
  } finally {
    if (currentSeq === usersTrendLoadSeq) {
      userTrendLoading.value = false
    }
  }
}

const loadUserSpendingRanking = async () => {
  const currentSeq = ++rankingLoadSeq
  rankingLoading.value = true
  rankingError.value = false
  try {
    const response = await adminAPI.dashboard.getUserSpendingRanking({
      start_date: startDate.value,
      end_date: endDate.value,
      limit: rankingLimit
    })
    if (currentSeq !== rankingLoadSeq) return
    rankingItems.value = response.ranking || []
    rankingTotalActualCost.value = response.total_actual_cost || 0
    rankingTotalRequests.value = response.total_requests || 0
    rankingTotalTokens.value = response.total_tokens || 0
  } catch (error) {
    if (currentSeq !== rankingLoadSeq) return
    console.error('Error loading user spending ranking:', error)
    rankingItems.value = []
    rankingTotalActualCost.value = 0
    rankingTotalRequests.value = 0
    rankingTotalTokens.value = 0
    rankingError.value = true
  } finally {
    if (currentSeq === rankingLoadSeq) {
      rankingLoading.value = false
    }
  }
}

const loadDashboardStats = async () => {
  await Promise.all([
    loadDashboardSnapshot(true),
    loadUsersTrend(),
    loadUserSpendingRanking()
  ])
}

const loadChartData = async () => {
  await Promise.all([
    loadDashboardSnapshot(false),
    loadUsersTrend(),
    loadUserSpendingRanking()
  ])
}

onMounted(() => {
  loadDashboardStats()
})
</script>

<style scoped>
</style>
