<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- BestAiPrice Styled Hero Banner -->
      <div class="bap-card relative overflow-hidden p-6 sm:p-7">
        <div class="flex flex-col justify-between gap-4 md:flex-row md:items-center">
          <div class="space-y-2">
            <div class="flex flex-wrap items-center gap-2.5">
              <span class="inline-flex items-center gap-1.5 rounded-full bg-emerald-50 px-2.5 py-0.5 text-xs font-semibold text-emerald-600 dark:bg-emerald-950/40 dark:text-emerald-400">
                <span class="relative flex h-2 w-2">
                  <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75"></span>
                  <span class="relative inline-flex h-2 w-2 rounded-full bg-emerald-500"></span>
                </span>
                Gateway Operational
              </span>
              <span v-if="user?.role" class="bap-pill bap-pill-blue uppercase text-[10px]">
                {{ user.role }}
              </span>
            </div>
            <h1 class="text-xl font-bold tracking-tight text-gray-900 sm:text-2xl dark:text-white">
              {{ t('dashboard.title') }}
            </h1>
            <p class="text-xs text-gray-500 sm:text-sm dark:text-gray-400">
              {{ t('dashboard.welcomeBack', { email: user?.email || user?.username || 'User' }) }}
              {{ t('dashboard.heroHint') }}
            </p>
          </div>

          <!-- Quick Action Shortcuts -->
          <div class="flex flex-wrap items-center gap-2.5">
            <router-link to="/keys" class="bap-btn-accent">
              <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
              </svg>
              <span>{{ t('dashboard.createApiKey') }}</span>
            </router-link>
            <router-link
              to="/usage"
              class="inline-flex items-center gap-1.5 rounded-xl border border-black/[0.08] bg-white px-3.5 py-2 text-xs font-semibold text-gray-700 shadow-sm transition-all hover:bg-gray-50 dark:border-white/[0.08] dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
            >
              <span>{{ t('dashboard.viewUsage') }}</span>
              <span class="text-gray-400">&rarr;</span>
            </router-link>
          </div>
        </div>
      </div>

      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-16">
        <LoadingSpinner size="lg" />
      </div>

      <template v-else>
        <!-- Instructions first: the 3 getting-started steps + the first request -->
        <UserDashboardOnboarding
          :has-key="hasKey"
          :has-balance="hasBalance"
          :has-request="hasRequest"
          :first-key="firstKey"
        />

        <template v-if="stats">
          <!-- Core & Token Stats -->
          <UserDashboardStats
            :stats="stats"
            :balance="user?.balance || 0"
            :is-simple="authStore.isSimpleMode"
          />

          <!-- Charts (Model Distribution & Token Usage Trend) -->
          <UserDashboardCharts
            v-model:startDate="startDate"
            v-model:endDate="endDate"
            v-model:granularity="granularity"
            :loading="loadingCharts"
            :trend="trendData"
            :models="modelStats"
            @dateRangeChange="loadCharts"
            @granularityChange="loadCharts"
          />

          <!-- Bottom Grid: Recent Usage (2 cols) & Quick Actions (1 col) -->
          <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <div class="lg:col-span-2">
              <UserDashboardRecentUsage :data="recentUsage" :loading="loadingUsage" />
            </div>
            <div class="lg:col-span-1">
              <UserDashboardQuickActions />
            </div>
          </div>
        </template>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { keysAPI } from '@/api'
import { usageAPI, type UserDashboardStats as UserStatsType } from '@/api/usage'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import UserDashboardOnboarding from '@/components/user/dashboard/UserDashboardOnboarding.vue'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'
import UserDashboardCharts from '@/components/user/dashboard/UserDashboardCharts.vue'
import UserDashboardRecentUsage from '@/components/user/dashboard/UserDashboardRecentUsage.vue'
import UserDashboardQuickActions from '@/components/user/dashboard/UserDashboardQuickActions.vue'
import type { ApiKey, UsageLog, TrendDataPoint, ModelStat } from '@/types'

const { t } = useI18n()
const authStore = useAuthStore()
const user = computed(() => authStore.user)
const stats = ref<UserStatsType | null>(null)
const keys = ref<ApiKey[]>([])
const loading = ref(false)
const loadingUsage = ref(false)
const loadingCharts = ref(false)
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const recentUsage = ref<UsageLog[]>([])

const formatLD = (d: Date) => d.toISOString().split('T')[0]
const startDate = ref(formatLD(new Date(Date.now() - 6 * 86400000)))
const endDate = ref(formatLD(new Date()))
const granularity = ref('day')

// Setup progress: a key exists, the balance is funded, a request went through.
const hasKey = computed(() => (stats.value?.total_api_keys ?? 0) > 0)
const hasBalance = computed(() => (user.value?.balance ?? 0) > 0)
const hasRequest = computed(() => (stats.value?.total_requests ?? 0) > 0)
const firstKey = computed<ApiKey | null>(() => keys.value[0] ?? null)

const loadStats = async () => {
  loading.value = true
  try {
    await authStore.refreshUser()
    const [dashboardStats, keyList] = await Promise.all([
      usageAPI.getDashboardStats(),
      keysAPI.list(1, 1).catch(() => null)
    ])
    stats.value = dashboardStats
    keys.value = keyList?.items ?? []
  } catch (error) {
    console.error('Failed to load dashboard stats:', error)
  } finally {
    loading.value = false
  }
}

const loadCharts = async () => {
  loadingCharts.value = true
  try {
    const res = await Promise.all([
      usageAPI.getDashboardTrend({
        start_date: startDate.value,
        end_date: endDate.value,
        granularity: granularity.value as any
      }),
      usageAPI.getDashboardModels({
        start_date: startDate.value,
        end_date: endDate.value
      })
    ])
    trendData.value = res[0].trend || []
    modelStats.value = res[1].models || []
  } catch (error) {
    console.error('Failed to load charts:', error)
  } finally {
    loadingCharts.value = false
  }
}

const loadRecent = async () => {
  loadingUsage.value = true
  try {
    const res = await usageAPI.getByDateRange(startDate.value, endDate.value)
    recentUsage.value = res.items.slice(0, 6)
  } catch (error) {
    console.error('Failed to load recent usage:', error)
  } finally {
    loadingUsage.value = false
  }
}

onMounted(() => {
  loadStats()
  loadCharts()
  loadRecent()
})
</script>
