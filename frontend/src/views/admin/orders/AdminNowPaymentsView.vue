<template>
  <AppLayout>
    <div class="mx-auto max-w-3xl space-y-6">
      <!-- Header -->
      <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-xl font-bold text-gray-900 dark:text-white flex items-center gap-2">
            <span>NOWPayments</span>
            <span
              :class="[
                'badge text-xs',
                form.enabled ? 'badge-success' : 'badge-neutral'
              ]"
            >
              {{ form.enabled ? t('nowpayments.statusActive') : t('nowpayments.statusDisabled') }}
            </span>
          </h1>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('nowpayments.subtitle') }}
          </p>
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            @click="loadData"
            :disabled="loading"
            class="btn btn-secondary btn-sm"
            :title="t('common.refresh')"
          >
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            <span>{{ t('common.refresh') }}</span>
          </button>
        </div>
      </div>

      <!-- Loading State -->
      <div v-if="loading && !loaded" class="card flex items-center justify-center p-12">
        <div class="h-6 w-6 animate-spin rounded-full border-2 border-primary-500 border-t-transparent" />
      </div>

      <!-- Form Card -->
      <form v-else @submit.prevent="saveSettings" class="card divide-y divide-gray-100 dark:divide-dark-700">
        <!-- Main Toggle Section -->
        <div class="p-6 space-y-4">
          <div class="flex items-center justify-between">
            <div>
              <label class="font-medium text-gray-900 dark:text-white">
                {{ t('nowpayments.enablePayment') }}
              </label>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('nowpayments.enablePaymentHint') }}
              </p>
            </div>
            <ToggleSwitch
              :label="t('common.enabled')"
              :checked="form.enabled"
              @toggle="form.enabled = !form.enabled"
            />
          </div>

          <!-- Sandbox Toggle -->
          <div class="flex items-center justify-between pt-3 border-t border-gray-100 dark:border-dark-700">
            <div>
              <div class="flex items-center gap-2">
                <label class="font-medium text-gray-900 dark:text-white">
                  {{ t('nowpayments.sandboxMode') }}
                </label>
                <span v-if="form.sandbox" class="badge badge-warning text-[11px]">
                  TESTNET / SANDBOX
                </span>
              </div>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ form.sandbox
                  ? t('nowpayments.sandboxActiveHint')
                  : t('nowpayments.productionActiveHint') }}
              </p>
            </div>
            <ToggleSwitch
              :label="t('nowpayments.sandbox')"
              :checked="form.sandbox"
              @toggle="form.sandbox = !form.sandbox"
            />
          </div>
        </div>

        <!-- Credentials Section -->
        <div class="p-6 space-y-5">
          <h2 class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
            {{ t('nowpayments.apiCredentials') }}
          </h2>

          <!-- API Key -->
          <div>
            <label class="input-label">
              {{ t('nowpayments.apiKey') }}
              <span class="text-red-500">*</span>
            </label>
            <div class="relative mt-1">
              <input
                v-model="form.apiKey"
                :type="showApiKey ? 'text' : 'password'"
                class="input pr-10 font-mono text-xs"
                :placeholder="t('nowpayments.apiKeyPlaceholder')"
                required
                autocomplete="off"
              />
              <button
                type="button"
                @click="showApiKey = !showApiKey"
                class="absolute inset-y-0 right-0 flex items-center pr-3 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
              >
                <Icon :name="showApiKey ? 'eyeOff' : 'eye'" size="sm" />
              </button>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('nowpayments.apiKeyHint') }}
            </p>
          </div>

          <!-- IPN Secret Key -->
          <div>
            <label class="input-label">
              {{ t('nowpayments.ipnSecret') }}
              <span class="text-red-500">*</span>
            </label>
            <div class="relative mt-1">
              <input
                v-model="form.ipnSecret"
                :type="showIpnSecret ? 'text' : 'password'"
                class="input pr-10 font-mono text-xs"
                :placeholder="t('nowpayments.ipnSecretPlaceholder')"
                required
                autocomplete="off"
              />
              <button
                type="button"
                @click="showIpnSecret = !showIpnSecret"
                class="absolute inset-y-0 right-0 flex items-center pr-3 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
              >
                <Icon :name="showIpnSecret ? 'eyeOff' : 'eye'" size="sm" />
              </button>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('nowpayments.ipnSecretHint') }}
            </p>
          </div>

          <!-- Currency -->
          <div>
            <label class="input-label">
              {{ t('nowpayments.currency') }}
            </label>
            <input
              v-model="form.currency"
              type="text"
              class="input font-mono text-sm max-w-xs uppercase"
              placeholder="USD"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('nowpayments.currencyHint') }}
            </p>
          </div>
        </div>

        <!-- Webhook (IPN) Instructions Card -->
        <div class="p-6 space-y-3 bg-gray-50/50 dark:bg-dark-800/30">
          <div class="flex items-center justify-between">
            <div>
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('nowpayments.ipnCallbackUrl') }}
              </h3>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('nowpayments.ipnCallbackUrlDesc') }}
              </p>
            </div>
            <a
              :href="dashboardUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="text-xs text-primary-600 hover:underline flex items-center gap-1 dark:text-primary-400"
            >
              <span>{{ t('nowpayments.openDashboard') }}</span>
              <svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"/></svg>
            </a>
          </div>

          <div class="flex items-center gap-2">
            <input
              :value="webhookUrl"
              readonly
              class="input font-mono text-xs bg-white dark:bg-dark-800"
            />
            <button
              type="button"
              @click="copyWebhookUrl"
              class="btn btn-secondary whitespace-nowrap text-xs"
            >
              {{ copied ? t('common.copied') : t('common.copy') }}
            </button>
          </div>
        </div>

        <!-- Footer Actions -->
        <div class="p-6 flex items-center justify-end gap-3 bg-gray-50/30 dark:bg-dark-800/20">
          <button
            type="submit"
            :disabled="saving"
            class="btn btn-primary"
          >
            <Icon v-if="saving" name="refresh" size="sm" class="animate-spin" />
            <span>{{ saving ? t('common.saving') : t('common.save') }}</span>
          </button>
        </div>
      </form>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ToggleSwitch from '@/components/payment/ToggleSwitch.vue'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const loaded = ref(false)
const saving = ref(false)
const copied = ref(false)

const showApiKey = ref(false)
const showIpnSecret = ref(false)

const providerId = ref<number | null>(null)

const form = reactive({
  enabled: false,
  sandbox: false,
  apiKey: '',
  ipnSecret: '',
  currency: 'USD',
})

const webhookUrl = computed(() => {
  if (typeof window !== 'undefined') {
    return `${window.location.origin}/api/v1/payment/webhook/nowpayments`
  }
  return '/api/v1/payment/webhook/nowpayments'
})

const dashboardUrl = computed(() => {
  return form.sandbox
    ? 'https://account-sandbox.nowpayments.io/store-settings'
    : 'https://account.nowpayments.io/store-settings'
})

async function loadData() {
  loading.value = true
  try {
    const res = await adminPaymentAPI.getProviders()
    const providers = Array.isArray(res.data) ? res.data : []
    const np = providers.find(p => p.provider_key === 'nowpayments')

    if (np) {
      providerId.value = np.id
      form.enabled = np.enabled
      form.apiKey = np.config?.apiKey || ''
      form.ipnSecret = np.config?.ipnSecret || ''
      form.sandbox = np.config?.sandbox === 'true' || np.config?.sandbox === '1'
      form.currency = np.config?.currency || 'USD'
    } else {
      providerId.value = null
      form.enabled = false
      form.sandbox = true
      form.apiKey = ''
      form.ipnSecret = ''
      form.currency = 'USD'
    }
    loaded.value = true
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    loading.value = false
  }
}

async function copyWebhookUrl() {
  try {
    await navigator.clipboard.writeText(webhookUrl.value)
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 2000)
  } catch {
    // Fallback
  }
}

async function saveSettings() {
  saving.value = true
  try {
    const configData: Record<string, string> = {
      apiKey: form.apiKey.trim(),
      ipnSecret: form.ipnSecret.trim(),
      sandbox: form.sandbox ? 'true' : 'false',
      currency: (form.currency.trim() || 'USD').toUpperCase(),
    }

    const payload = {
      provider_key: 'nowpayments',
      name: form.sandbox ? 'NOWPayments (Sandbox)' : 'NOWPayments (Crypto)',
      supported_types: ['nowpayments'],
      enabled: form.enabled,
      payment_mode: 'redirect',
      refund_enabled: false,
      allow_user_refund: false,
      config: configData,
      limits: '',
    }

    if (providerId.value) {
      await adminPaymentAPI.updateProvider(providerId.value, payload)
    } else {
      const res = await adminPaymentAPI.createProvider(payload)
      if (res.data?.id) {
        providerId.value = res.data.id
      }
    }

    // Sync global payment master switch
    await adminPaymentAPI.updateConfig({ enabled: form.enabled })
    await appStore.fetchPublicSettings()

    appStore.showSuccess(t('nowpayments.savedSuccess'))
    await loadData()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  loadData()
})
</script>
