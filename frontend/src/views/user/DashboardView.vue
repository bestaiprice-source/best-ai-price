<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- BestAiPrice Styled Hero Banner -->
      <div class="bap-card relative overflow-hidden p-6 sm:p-7">
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
      </div>

      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-16">
        <LoadingSpinner size="lg" />
      </div>

      <!-- Getting started: 3 steps + the first request example -->
      <UserDashboardOnboarding
        v-else
        :has-key="hasKey"
        :has-balance="hasBalance"
        :has-request="hasRequest"
        :first-key="firstKey"
      />
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
import type { ApiKey } from '@/types'

const { t } = useI18n()
const authStore = useAuthStore()
const user = computed(() => authStore.user)
const stats = ref<UserStatsType | null>(null)
const keys = ref<ApiKey[]>([])
const loading = ref(false)

// Setup progress: a key exists, the balance is funded, a request went through.
const hasKey = computed(() => (stats.value?.total_api_keys ?? 0) > 0)
const hasBalance = computed(() => (user.value?.balance ?? 0) > 0)
const hasRequest = computed(() => (stats.value?.total_requests ?? 0) > 0)
const firstKey = computed<ApiKey | null>(() => keys.value[0] ?? null)

const load = async () => {
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
    console.error('Failed to load dashboard setup state:', error)
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
