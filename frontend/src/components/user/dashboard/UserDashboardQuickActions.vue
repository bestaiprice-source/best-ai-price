<template>
  <div class="bap-card p-5">
    <div class="mb-4 flex items-center justify-between border-b border-black/[0.04] pb-3 dark:border-white/[0.06]">
      <h2 class="text-sm font-semibold tracking-tight text-gray-900 dark:text-white">
        {{ t('dashboard.quickActions') }}
      </h2>
      <span class="bap-pill bap-pill-gray text-[10px]">{{
        t('ui.userDashboardQuickActions.shortcuts')
      }}</span>
    </div>

    <div class="space-y-2.5">
      <!-- Create API Key -->
      <button
        @click="router.push('/keys')"
        class="group flex w-full items-center gap-3.5 rounded-xl border border-black/[0.04] bg-white p-3.5 text-left shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:border-black/15 hover:shadow-md dark:border-white/[0.06] dark:bg-[#161b22] dark:hover:border-white/15"
      >
        <div class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-[#e8f2ff] text-[#0355cd] transition-transform group-hover:scale-110 dark:bg-[#0355cd]/25 dark:text-[#60a5fa]">
          <Icon name="key" size="md" :stroke-width="2" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-xs font-semibold text-gray-900 dark:text-white">
            {{ t('dashboard.createApiKey') }}
          </p>
          <p class="text-[11px] text-gray-400 dark:text-gray-500">
            {{ t('dashboard.generateNewKey') }}
          </p>
        </div>
        <Icon
          name="chevronRight"
          size="sm"
          class="text-gray-300 transition-all group-hover:translate-x-1 group-hover:text-[#4791ff] dark:text-gray-600"
        />
      </button>

      <!-- View Detailed Usage -->
      <button
        @click="router.push('/usage')"
        class="group flex w-full items-center gap-3.5 rounded-xl border border-black/[0.04] bg-white p-3.5 text-left shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:border-black/15 hover:shadow-md dark:border-white/[0.06] dark:bg-[#161b22] dark:hover:border-white/15"
      >
        <div class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-[#e7f9f2] text-[#10b981] transition-transform group-hover:scale-110 dark:bg-[#10b981]/20 dark:text-[#34d399]">
          <Icon name="chart" size="md" :stroke-width="2" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-xs font-semibold text-gray-900 dark:text-white">
            {{ t('dashboard.viewUsage') }}
          </p>
          <p class="text-[11px] text-gray-400 dark:text-gray-500">
            {{ t('dashboard.checkDetailedLogs') }}
          </p>
        </div>
        <Icon
          name="chevronRight"
          size="sm"
          class="text-gray-300 transition-all group-hover:translate-x-1 group-hover:text-emerald-500 dark:text-gray-600"
        />
      </button>

      <!-- Redeem Code -->
      <button
        @click="router.push('/redeem')"
        class="group flex w-full items-center gap-3.5 rounded-xl border border-black/[0.04] bg-white p-3.5 text-left shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:border-black/15 hover:shadow-md dark:border-white/[0.06] dark:bg-[#161b22] dark:hover:border-white/15"
      >
        <div class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-[#fef3c7] text-[#d97706] transition-transform group-hover:scale-110 dark:bg-[#d97706]/20 dark:text-[#fbbf24]">
          <Icon name="gift" size="md" :stroke-width="2" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-xs font-semibold text-gray-900 dark:text-white">
            {{ t('dashboard.redeemCode') }}
          </p>
          <p class="text-[11px] text-gray-400 dark:text-gray-500">
            {{ t('dashboard.addBalanceWithCode') }}
          </p>
        </div>
        <Icon
          name="chevronRight"
          size="sm"
          class="text-gray-300 transition-all group-hover:translate-x-1 group-hover:text-amber-500 dark:text-gray-600"
        />
      </button>

      <!-- Subscriptions & Plans -->
      <button
        @click="router.push('/subscriptions')"
        class="group flex w-full items-center gap-3.5 rounded-xl border border-black/[0.04] bg-white p-3.5 text-left shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:border-black/15 hover:shadow-md dark:border-white/[0.06] dark:bg-[#161b22] dark:hover:border-white/15"
      >
        <div class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-[#f3e8ff] text-[#9333ea] transition-transform group-hover:scale-110 dark:bg-[#9333ea]/20 dark:text-[#c084fc]">
          <Icon name="bolt" size="md" :stroke-width="2" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-xs font-semibold text-gray-900 dark:text-white">
            {{ t('ui.userDashboardQuickActions.topUpPlans') }}
          </p>
          <p class="text-[11px] text-gray-400 dark:text-gray-500">
            {{ t('ui.userDashboardQuickActions.managequotabilling') }}
          </p>
        </div>
        <Icon
          name="chevronRight"
          size="sm"
          class="text-gray-300 transition-all group-hover:translate-x-1 group-hover:text-purple-500 dark:text-gray-600"
        />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const router = useRouter()
const { t } = useI18n()
</script>
