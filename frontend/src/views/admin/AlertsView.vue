<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Header -->
      <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-white">{{ t('ui.alertsView.alerts') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            Upstream alerting: Telegram notifications, thresholds, deduplication cooldowns and recent incidents.
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <button
            class="inline-flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 shadow-sm transition-all hover:bg-gray-50 disabled:opacity-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200"
            :disabled="testing"
            @click="sendTest"
          >
            <Icon name="bell" size="sm" />
            <span>{{ testing ? t('ui.alertsView.sending') : t('ui.alertsView.sendTestNotification') }}</span>
          </button>
          <router-link
            to="/admin/settings"
            class="inline-flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 shadow-sm transition-all hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200"
          >
            <Icon name="cog" size="sm" />
            <span>{{ t('ui.alertsView.telegramsettings') }}</span>
          </router-link>
          <router-link
            to="/admin/ops"
            class="inline-flex items-center gap-2 rounded-xl bg-primary-600 px-4 py-2.5 text-sm font-medium text-white shadow-sm transition-all hover:bg-primary-500"
          >
            <Icon name="chart" size="sm" />
            <span>{{ t('ui.alertsView.opsalertrules') }}</span>
          </router-link>
        </div>
      </div>

      <!-- Telegram summary -->
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('ui.alertsView.telegramalerts') }}</p>
          <p class="mt-2 text-2xl font-bold" :class="telegram?.alerts_enabled ? 'text-emerald-600' : 'text-gray-400'">
            {{ telegram?.alerts_enabled ? 'Enabled' : 'Disabled' }}
          </p>
        </div>
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('ui.alertsView.botconfigured') }}</p>
          <p class="mt-2 text-2xl font-bold" :class="telegram?.bot_token_configured ? 'text-emerald-600' : 'text-red-500'">
            {{ telegram?.bot_token_configured ? 'Yes' : 'No' }}
          </p>
        </div>
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('ui.alertsView.balancethreshold') }}</p>
          <p class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">
            {{ telegram ? '$' + Number(telegram.alert_threshold_usd || 0).toFixed(2) : '—' }}
          </p>
        </div>
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('ui.alertsView.recipients') }}</p>
          <p class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ telegram?.active_recipients?.length ?? 0 }}</p>
        </div>
      </div>

      <!-- Upstream event types -->
      <div class="overflow-hidden rounded-2xl border border-gray-200/80 bg-white/85 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
        <div class="border-b border-gray-200 px-5 py-3 dark:border-dark-700">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('ui.alertsView.upstreamalerteventtypes') }}</h2>
        </div>
        <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50/60 text-left text-xs uppercase tracking-wider text-gray-500 dark:bg-dark-900/40 dark:text-gray-400">
            <tr>
              <th class="px-5 py-3">{{ t('ui.alertsView.event') }}</th>
              <th class="px-5 py-3">{{ t('ui.alertsView.description') }}</th>
              <th class="px-5 py-3 text-right">{{ t('ui.alertsView.cooldown') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-50 dark:divide-dark-800">
            <tr v-for="ev in eventTypes" :key="ev.key">
              <td class="px-5 py-3 font-medium text-gray-900 dark:text-white">{{ ev.name }}</td>
              <td class="px-5 py-3 text-gray-500 dark:text-gray-400">{{ ev.desc }}</td>
              <td class="px-5 py-3 text-right text-gray-500 dark:text-gray-400">{{ ev.cooldown }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <!-- Alert rules -->
        <div class="overflow-hidden rounded-2xl border border-gray-200/80 bg-white/85 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <div class="border-b border-gray-200 px-5 py-3 dark:border-dark-700">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('ui.alertsView.opsalertrules2') }}</h2>
          </div>
          <div class="max-h-[420px] overflow-y-auto">
            <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
              <thead class="sticky top-0 bg-gray-50/90 text-left text-xs uppercase tracking-wider text-gray-500 dark:bg-dark-900 dark:text-gray-400">
                <tr>
                  <th class="px-4 py-2">{{ t('ui.alertsView.rule') }}</th>
                  <th class="px-4 py-2">{{ t('ui.alertsView.metric') }}</th>
                  <th class="px-4 py-2 text-right">{{ t('ui.alertsView.threshold') }}</th>
                  <th class="px-4 py-2">{{ t('ui.alertsView.severity') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-50 dark:divide-dark-800">
                <tr v-for="r in rules" :key="r.id">
                  <td class="px-4 py-2">
                    <div class="font-medium text-gray-900 dark:text-white">{{ metricLabel(r.metric_type) }}</div>
                    <div v-if="!isLatinText(r.name)" class="text-xs text-gray-400">{{ r.name }}</div>
                    <div class="text-xs" :class="r.enabled ? 'text-emerald-500' : 'text-gray-400'">{{ r.enabled ? 'enabled' : 'disabled' }}</div>
                  </td>
                  <td class="px-4 py-2 text-xs text-gray-500 dark:text-gray-400">{{ r.metric_type }}</td>
                  <td class="px-4 py-2 text-right text-gray-700 dark:text-gray-300">{{ r.operator }} {{ r.threshold }}</td>
                  <td class="px-4 py-2"><span :class="severityBadgeClass(r.severity)">{{ r.severity }}</span></td>
                </tr>
                <tr v-if="!loading && rules.length === 0">
                  <td colspan="4" class="px-4 py-8 text-center text-gray-400">{{ t('ui.alertsView.noalertrulesConfigure') }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- Recent alert events -->
        <div class="overflow-hidden rounded-2xl border border-gray-200/80 bg-white/85 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <div class="border-b border-gray-200 px-5 py-3 dark:border-dark-700">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('ui.alertsView.recentalertevents') }}</h2>
          </div>
          <div class="max-h-[420px] overflow-y-auto divide-y divide-gray-50 dark:divide-dark-800">
            <div v-for="ev in events" :key="ev.id" class="px-5 py-3">
              <div class="flex items-center justify-between gap-3">
                <span :class="severityBadgeClass(ev.severity)">{{ ev.severity }}</span>
                <span class="text-xs text-gray-400">{{ formatTime(ev.fired_at || ev.created_at) }}</span>
              </div>
              <p class="mt-1 text-sm text-gray-800 dark:text-gray-200">{{ eventLabel(ev) }}</p>
              <p v-if="!isLatinText(ev.title) && ev.title" class="text-xs text-gray-400">{{ ev.title }}</p>
              <p class="text-xs text-gray-400">status: {{ ev.status }}</p>
            </div>
            <div v-if="!loading && events.length === 0" class="px-5 py-8 text-center text-gray-400">
              {{ t('ui.alertsView.noalerteventsrecorded') }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { getTelegramSettings, testTelegramNotification, type TelegramSettings } from '@/api/admin/settings'
import { listAlertRules, listAlertEvents, type AlertRule, type AlertEvent } from '@/api/admin/ops'
import { metricLabel, isLatinText } from '@/utils/opsLabels'

const appStore = useAppStore()
const { t } = useI18n()

const loading = ref(false)
const testing = ref(false)
const telegram = ref<TelegramSettings | null>(null)
const rules = ref<AlertRule[]>([])
const events = ref<AlertEvent[]>([])

// Reference of the upstream alert event types emitted by the Yunwu notifier.
const eventTypes = [
  { key: 'provider_down_degraded', name: 'Provider Down / Degraded', desc: 'Node or pool failed health checks or entered cooldown.', cooldown: '30 min' },
  { key: 'upstream_key_invalid', name: 'Upstream API key invalid/disabled', desc: 'Upstream returned 401/403 or a disabled-key error.', cooldown: '1 h' },
  { key: 'balance_low', name: 'Provider balance low', desc: 'Provider credit dropped below its threshold.', cooldown: '12 h' },
  { key: 'both_providers_unavailable', name: 'Both providers unavailable', desc: 'No compatible candidate node could serve the model.', cooldown: '30 min' },
  { key: 'failover_rate_exceeded', name: 'Failover rate exceeded', desc: 'Model failover rate crossed the threshold in the window.', cooldown: '30 min' },
  { key: 'error_spike_429_5xx', name: 'Surge in 429 / 5xx errors', desc: 'Error rate crossed the threshold in the window.', cooldown: '15 min' },
  { key: 'cheaper_provider_found', name: 'Cheaper provider found', desc: 'A cheaper alternative provider was detected.', cooldown: '24 h' },
  { key: 'price_changed_significantly', name: 'Provider price changed significantly', desc: 'Upstream price moved enough to require a retail recalculation.', cooldown: '24 h' }
]

const formatTime = (v?: string) => (v ? new Date(v).toLocaleString() : '—')

// Map rule_id → metric_type so alert events (whose stored title may be a
// free-form/Chinese string) can be shown with a stable English label.
const ruleMetricMap = ref<Map<number, string>>(new Map())
const eventLabel = (ev: AlertEvent): string => {
  const mt = ruleMetricMap.value.get(ev.rule_id)
  if (mt) return metricLabel(mt)
  return ev.title || ev.description || ('Event #' + ev.id)
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
    const [tgRes, rulesRes, eventsRes] = await Promise.allSettled([
      getTelegramSettings(),
      listAlertRules(),
      listAlertEvents({ limit: 30 })
    ])
    if (tgRes.status === 'fulfilled') telegram.value = tgRes.value
    if (rulesRes.status === 'fulfilled') {
      rules.value = Array.isArray(rulesRes.value) ? rulesRes.value : []
      const map = new Map<number, string>()
      for (const r of rules.value) {
        if (r.id != null && r.metric_type) map.set(r.id, r.metric_type)
      }
      ruleMetricMap.value = map
    }
    if (eventsRes.status === 'fulfilled') {
      const list = eventsRes.value as AlertEvent[]
      events.value = Array.isArray(list) ? list.slice(0, 30) : []
    }
  } catch (e) {
    console.warn('Failed to load alerts:', e)
    appStore.showError('Failed to load alerts')
  } finally {
    loading.value = false
  }
}

const sendTest = async () => {
  testing.value = true
  try {
    const res = await testTelegramNotification()
    appStore.showSuccess(`Test notification sent to ${res.sent_count} recipient(s)`)
  } catch (e) {
    console.warn('Telegram test failed:', e)
    appStore.showError('Test notification failed')
  } finally {
    testing.value = false
  }
}

onMounted(load)
</script>
