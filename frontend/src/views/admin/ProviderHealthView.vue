<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Header -->
      <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-white">{{ t('ui.providerHealthView.providerHealth') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            Live upstream availability: pool state, latency, error rate, failover and recent incidents.
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <button
            class="inline-flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 shadow-sm transition-all hover:bg-gray-50 disabled:opacity-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200"
            :disabled="loading"
            @click="load"
          >
            <Icon name="sync" size="sm" :class="{ 'animate-spin': loading }" />
            <span>{{ t('ui.providerHealthView.refresh') }}</span>
          </button>
          <router-link
            to="/admin/providers"
            class="inline-flex items-center gap-2 rounded-xl bg-primary-600 px-4 py-2.5 text-sm font-medium text-white shadow-sm transition-all hover:bg-primary-500"
          >
            <Icon name="globe" size="sm" />
            <span>{{ t('ui.providerHealthView.manageProviders') }}</span>
          </router-link>
        </div>
      </div>

      <!-- Pool status -->
      <div
        v-if="pool"
        class="rounded-2xl border p-5"
        :class="poolBannerClass"
      >
        <div class="flex flex-wrap items-center justify-between gap-4">
          <div>
            <p class="text-xs font-semibold uppercase tracking-wider opacity-70">{{ t('ui.providerHealthView.yunwupoolstate') }}</p>
            <p class="mt-1 text-2xl font-bold">{{ pool.system_state }}</p>
          </div>
          <div class="grid grid-cols-2 gap-x-8 gap-y-2 text-sm sm:grid-cols-4">
            <div>
              <p class="opacity-70">{{ t('ui.providerHealthView.healthynodes') }}</p>
              <p class="text-lg font-semibold">{{ pool.healthy_nodes }} / {{ pool.total_nodes }}</p>
            </div>
            <div>
              <p class="opacity-70">{{ t('ui.providerHealthView.incooldown') }}</p>
              <p class="text-lg font-semibold">{{ pool.cooldown_nodes }}</p>
            </div>
            <div>
              <p class="opacity-70">{{ t('ui.providerHealthView.rebinds24h') }}</p>
              <p class="text-lg font-semibold">{{ pool.rebinding_24h_cnt ?? 0 }}</p>
            </div>
            <div>
              <p class="opacity-70">{{ t('ui.providerHealthView.allocatable') }}</p>
              <p class="text-lg font-semibold">{{ formatUsd(pool.total_allocatable_usd) }}</p>
            </div>
          </div>
        </div>
      </div>

      <!-- KPIs -->
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('ui.providerHealthView.providers') }}</p>
          <p class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ providers.length }}</p>
        </div>
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('ui.providerHealthView.healthy') }}</p>
          <p class="mt-2 text-2xl font-bold text-emerald-600 dark:text-emerald-400">{{ healthyCount }}</p>
        </div>
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('ui.providerHealthView.degradedDown') }}</p>
          <p class="mt-2 text-2xl font-bold text-amber-600 dark:text-amber-400">{{ degradedCount }}</p>
        </div>
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('ui.providerHealthView.lowbalance') }}</p>
          <p class="mt-2 text-2xl font-bold text-red-600 dark:text-red-400">{{ lowBalanceCount }}</p>
        </div>
      </div>

      <!-- Providers health table -->
      <div class="overflow-hidden rounded-2xl border border-gray-200/80 bg-white/85 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
        <div class="border-b border-gray-200 px-5 py-3 dark:border-dark-700">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('ui.providerHealthView.providermetrics') }}</h2>
        </div>
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50/60 dark:bg-dark-900/40">
              <tr class="text-left text-xs uppercase tracking-wider text-gray-500 dark:text-gray-400">
                <th class="px-5 py-3">{{ t('ui.providerHealthView.provider') }}</th>
                <th class="px-5 py-3">{{ t('ui.providerHealthView.state') }}</th>
                <th class="px-5 py-3">{{ t('ui.providerHealthView.families') }}</th>
                <th class="px-5 py-3 text-right">{{ t('ui.providerHealthView.latency') }}</th>
                <th class="px-5 py-3 text-right">{{ t('ui.providerHealthView.errorrate') }}</th>
                <th class="px-5 py-3 text-right">{{ t('ui.providerHealthView.requests') }}</th>
                <th class="px-5 py-3 text-right">{{ t('ui.providerHealthView.spend') }}</th>
                <th class="px-5 py-3 text-right">{{ t('ui.providerHealthView.balance') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-50 dark:divide-dark-800">
              <tr v-for="p in providers" :key="p.id">
                <td class="px-5 py-3">
                  <div class="font-medium text-gray-900 dark:text-white">{{ p.provider_name || ('#' + p.id) }}</div>
                  <div class="text-xs text-gray-400">{{ p.domain || p.base_url }}</div>
                </td>
                <td class="px-5 py-3"><span :class="stateBadgeClass(p.state)">{{ p.state }}</span></td>
                <td class="px-5 py-3">
                  <span class="text-xs text-gray-500 dark:text-gray-400">{{ (p.supported_model_families || []).join(', ') || '—' }}</span>
                </td>
                <td class="px-5 py-3 text-right">{{ p.metrics?.latency_ms ? p.metrics.latency_ms + ' ms' : '—' }}</td>
                <td class="px-5 py-3 text-right">
                  <span :class="p.metrics?.error_rate > 10 ? 'text-red-600 dark:text-red-400' : ''">
                    {{ formatPercent(p.metrics?.error_rate) }}
                  </span>
                </td>
                <td class="px-5 py-3 text-right">{{ p.metrics?.requests ?? 0 }}</td>
                <td class="px-5 py-3 text-right">{{ formatUsd(p.metrics?.spend_usd) }}</td>
                <td class="px-5 py-3 text-right" :class="p.state === 'Low balance' ? 'text-red-600 dark:text-red-400' : ''">
                  {{ formatUsd(p.balance_usd) }}
                </td>
              </tr>
              <tr v-if="!loading && providers.length === 0">
                <td colspan="8" class="px-5 py-8 text-center text-gray-400">{{ t('ui.providerHealthView.noprovidersconfigured') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <!-- Nodes -->
        <div class="overflow-hidden rounded-2xl border border-gray-200/80 bg-white/85 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <div class="border-b border-gray-200 px-5 py-3 dark:border-dark-700">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('ui.providerHealthView.executionnodes') }}</h2>
          </div>
          <div class="max-h-[420px] overflow-y-auto">
            <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
              <thead class="sticky top-0 bg-gray-50/90 text-left text-xs uppercase tracking-wider text-gray-500 dark:bg-dark-900 dark:text-gray-400">
                <tr>
                  <th class="px-4 py-2">{{ t('ui.providerHealthView.node') }}</th>
                  <th class="px-4 py-2">{{ t('ui.providerHealthView.state2') }}</th>
                  <th class="px-4 py-2 text-right">{{ t('ui.providerHealthView.allocatable2') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-50 dark:divide-dark-800">
                <tr v-for="n in nodes" :key="n.node_id">
                  <td class="px-4 py-2">
                    <div class="font-medium text-gray-900 dark:text-white">{{ n.account_name || n.node_id }}</div>
                    <div class="text-xs text-gray-400">{{ n.provider || n.platform }}</div>
                  </td>
                  <td class="px-4 py-2"><span :class="stateBadgeClass(mapNodeState(n.state))">{{ n.state }}</span></td>
                  <td class="px-4 py-2 text-right">{{ formatUsd(n.allocatable_usd ?? n.total_available_usd) }}</td>
                </tr>
                <tr v-if="!loading && nodes.length === 0">
                  <td colspan="3" class="px-4 py-8 text-center text-gray-400">{{ t('ui.providerHealthView.noexecutionnodes') }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- Recent incidents -->
        <div class="overflow-hidden rounded-2xl border border-gray-200/80 bg-white/85 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <div class="border-b border-gray-200 px-5 py-3 dark:border-dark-700">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('ui.providerHealthView.recentincidents') }}</h2>
          </div>
          <div class="max-h-[420px] overflow-y-auto divide-y divide-gray-50 dark:divide-dark-800">
            <div v-for="ev in incidents" :key="ev.id" class="px-5 py-3">
              <div class="flex items-center justify-between gap-3">
                <span :class="severityBadgeClass(ev.severity)">{{ ev.severity }}</span>
                <span class="text-xs text-gray-400">{{ formatTime(ev.fired_at || ev.created_at) }}</span>
              </div>
              <p class="mt-1 text-sm text-gray-800 dark:text-gray-200">{{ eventLabel(ev) }}</p>
              <p v-if="!isLatinText(ev.title) && ev.title" class="text-xs text-gray-400">{{ ev.title }}</p>
              <p class="text-xs text-gray-400">status: {{ ev.status }}</p>
            </div>
            <div v-if="!loading && incidents.length === 0" class="px-5 py-8 text-center text-gray-400">
              {{ t('ui.providerHealthView.noincidentsrecorded') }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { getProviders, type Provider } from '@/api/admin/providers'
import { getAshYunwuHealth, getAshYunwuNodes, type AshHealthResponse, type YunwuNodeItem } from '@/api/admin/ash'
import { listAlertEvents, listAlertRules, type AlertEvent } from '@/api/admin/ops'
import { metricLabel, isLatinText } from '@/utils/opsLabels'

const appStore = useAppStore()
const { t } = useI18n()

const loading = ref(false)
const providers = ref<Provider[]>([])
const pool = ref<AshHealthResponse | null>(null)
const nodes = ref<YunwuNodeItem[]>([])
const incidents = ref<AlertEvent[]>([])

const healthyCount = computed(() => providers.value.filter(p => p.state === 'Healthy').length)
const degradedCount = computed(() => providers.value.filter(p => p.state === 'Degraded' || p.state === 'Down').length)
const lowBalanceCount = computed(() => providers.value.filter(p => p.state === 'Low balance').length)

const poolBannerClass = computed(() => {
  switch ((pool.value?.system_state || 'NORMAL').toUpperCase()) {
    case 'NORMAL':
      return 'border-emerald-200 bg-emerald-50 text-emerald-900 dark:border-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-200'
    case 'WARNING':
      return 'border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-200'
    default:
      return 'border-red-200 bg-red-50 text-red-900 dark:border-red-800 dark:bg-red-950/40 dark:text-red-200'
  }
})

const formatUsd = (v?: number | null) => (typeof v === 'number' ? '$' + v.toFixed(2) : '—')
const formatPercent = (v?: number | null) => (typeof v === 'number' ? v.toFixed(1) + '%' : '—')
const formatTime = (v?: string) => (v ? new Date(v).toLocaleString() : '—')

// Map rule_id → metric_type so alert events with free-form/Chinese titles can be
// rendered with a stable English label.
const ruleMetricMap = ref<Map<number, string>>(new Map())
const eventLabel = (ev: AlertEvent): string => {
  const mt = ruleMetricMap.value.get(ev.rule_id)
  if (mt) return metricLabel(mt)
  return ev.title || ev.description || ('Event #' + ev.id)
}

const mapNodeState = (state: string) => {
  const s = (state || '').toLowerCase()
  if (s === 'healthy') return 'Healthy'
  if (s === 'cooldown' || s === 'limited') return 'Degraded'
  if (s === 'unavailable') return 'Down'
  return state
}

const stateBadgeClass = (state: string) => {
  const base = 'inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium '
  switch (state) {
    case 'Healthy':
      return base + 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
    case 'Degraded':
      return base + 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
    case 'Down':
    case 'Invalid key':
      return base + 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
    case 'Low balance':
      return base + 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
    default:
      return base + 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
}

const severityBadgeClass = (severity: string) => {
  const base = 'inline-flex rounded-full px-2 py-0.5 text-xs font-medium '
  switch ((severity || '').toLowerCase()) {
    case 'critical':
      return base + 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
    case 'warning':
      return base + 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
    default:
      return base + 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
  }
}

const load = async () => {
  loading.value = true
  try {
    const [provRes, poolRes, nodesRes, incidentsRes, rulesRes] = await Promise.allSettled([
      getProviders(1, 200),
      getAshYunwuHealth(),
      getAshYunwuNodes(),
      listAlertEvents({ limit: 20 }),
      listAlertRules()
    ])
    if (provRes.status === 'fulfilled') providers.value = provRes.value.items || []
    if (poolRes.status === 'fulfilled') pool.value = poolRes.value
    if (nodesRes.status === 'fulfilled') nodes.value = Array.isArray(nodesRes.value) ? nodesRes.value : []
    if (rulesRes.status === 'fulfilled') {
      const map = new Map<number, string>()
      for (const r of (rulesRes.value || [])) {
        if (r.id != null && r.metric_type) map.set(r.id, r.metric_type)
      }
      ruleMetricMap.value = map
    }
    if (incidentsRes.status === 'fulfilled') {
      const list = incidentsRes.value as AlertEvent[]
      incidents.value = Array.isArray(list) ? list.slice(0, 20) : []
    }
  } catch (e) {
    console.warn('Failed to load provider health:', e)
    appStore.showError('Failed to load provider health')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
