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
                'badge text-xs font-semibold',
                form.enabled ? 'badge-success' : 'badge-neutral'
              ]"
            >
              {{ form.enabled ? t('nowpayments.statusActive') : t('nowpayments.statusDisabled') }}
            </span>
            <span v-if="form.sandbox" class="badge badge-warning text-[11px] font-semibold">
              TESTNET / SANDBOX
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
        <!-- Status Banner -->
        <div
          v-if="providerId && form.enabled"
          class="p-4 bg-emerald-50 dark:bg-emerald-900/20 border-b border-emerald-100 dark:border-emerald-800/40 flex items-start gap-3"
        >
          <div class="mt-0.5 text-emerald-600 dark:text-emerald-400">
            <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
          </div>
          <div>
            <h4 class="text-sm font-semibold text-emerald-900 dark:text-emerald-200">
              {{ t('nowpayments.statusActive') }}
            </h4>
            <p class="text-xs text-emerald-700 dark:text-emerald-300 mt-0.5">
              {{ t('nowpayments.bannerActive') }}
            </p>
          </div>
        </div>

        <div
          v-else-if="providerId && !form.enabled"
          class="p-4 bg-amber-50 dark:bg-amber-900/20 border-b border-amber-100 dark:border-amber-800/40 flex items-start gap-3"
        >
          <div class="mt-0.5 text-amber-600 dark:text-amber-400">
            <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/></svg>
          </div>
          <div>
            <h4 class="text-sm font-semibold text-amber-900 dark:text-amber-200">
              {{ t('nowpayments.statusDisabled') }}
            </h4>
            <p class="text-xs text-amber-700 dark:text-amber-300 mt-0.5">
              {{ t('nowpayments.bannerSavedDisabled') }}
            </p>
          </div>
        </div>

        <!-- Main Toggles Section -->
        <div class="p-6 space-y-4">
          <!-- Enable Toggle -->
          <div class="flex items-center justify-between cursor-pointer py-1" @click="form.enabled = !form.enabled">
            <div class="pr-4">
              <div class="flex items-center gap-2">
                <span class="font-medium text-gray-900 dark:text-white">
                  {{ t('nowpayments.enablePayment') }}
                </span>
                <span
                  :class="[
                    'badge text-xs font-semibold',
                    form.enabled ? 'badge-success' : 'badge-neutral'
                  ]"
                >
                  {{ form.enabled ? t('nowpayments.statusActive') : t('nowpayments.statusDisabled') }}
                </span>
              </div>
              <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
                {{ t('nowpayments.enablePaymentHint') }}
              </p>
            </div>
            <button
              type="button"
              role="switch"
              :aria-checked="form.enabled"
              @click.stop="form.enabled = !form.enabled"
              :class="[
                'relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                form.enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow-lg ring-0 transition duration-200 ease-in-out',
                  form.enabled ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>

          <!-- Sandbox Toggle -->
          <div class="flex items-center justify-between cursor-pointer pt-3 border-t border-gray-100 dark:border-dark-700" @click="form.sandbox = !form.sandbox">
            <div class="pr-4">
              <div class="flex items-center gap-2">
                <span class="font-medium text-gray-900 dark:text-white">
                  {{ t('nowpayments.sandboxMode') }}
                </span>
                <span v-if="form.sandbox" class="badge badge-warning text-[11px] font-semibold">
                  TESTNET / SANDBOX
                </span>
                <span v-else class="badge badge-info text-[11px] font-semibold">
                  PRODUCTION
                </span>
              </div>
              <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
                {{ form.sandbox
                  ? t('nowpayments.sandboxActiveHint')
                  : t('nowpayments.productionActiveHint') }}
              </p>
            </div>
            <button
              type="button"
              role="switch"
              :aria-checked="form.sandbox"
              @click.stop="form.sandbox = !form.sandbox"
              :class="[
                'relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                form.sandbox ? 'bg-amber-500' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow-lg ring-0 transition duration-200 ease-in-out',
                  form.sandbox ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>
        </div>

        <!-- Credentials Section -->
        <div class="p-6 space-y-5">
          <h2 class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
            {{ t('nowpayments.apiCredentials') }}
          </h2>

          <!-- API Key -->
          <div>
            <div class="flex items-center justify-between">
              <label class="input-label mb-0">
                {{ t('nowpayments.apiKey') }}
                <span v-if="!hasApiKey" class="text-red-500">*</span>
              </label>
              <span v-if="hasApiKey" class="badge badge-success text-[11px] gap-1 flex items-center">
                <svg class="h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>
                {{ t('nowpayments.keyConfigured') }}
              </span>
            </div>
            <div class="relative mt-1">
              <input
                v-model="form.apiKey"
                :type="showApiKey ? 'text' : 'password'"
                class="input pr-10 font-mono text-xs"
                :placeholder="hasApiKey ? '•••••••••••••••••••••••••••••••• (' + t('nowpayments.leaveEmptyToKeep') + ')' : t('nowpayments.apiKeyPlaceholder')"
                :required="!hasApiKey"
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
              {{ hasApiKey ? t('nowpayments.apiKeyConfiguredHint') : t('nowpayments.apiKeyHint') }}
            </p>
          </div>

          <!-- IPN Secret Key -->
          <div>
            <div class="flex items-center justify-between">
              <label class="input-label mb-0">
                {{ t('nowpayments.ipnSecret') }}
                <span v-if="!hasIpnSecret" class="text-red-500">*</span>
              </label>
              <span v-if="hasIpnSecret" class="badge badge-success text-[11px] gap-1 flex items-center">
                <svg class="h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>
                {{ t('nowpayments.keyConfigured') }}
              </span>
            </div>
            <div class="relative mt-1">
              <input
                v-model="form.ipnSecret"
                :type="showIpnSecret ? 'text' : 'password'"
                class="input pr-10 font-mono text-xs"
                :placeholder="hasIpnSecret ? '•••••••••••••••••••••••••••••••• (' + t('nowpayments.leaveEmptyToKeep') + ')' : t('nowpayments.ipnSecretPlaceholder')"
                :required="!hasIpnSecret"
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
              {{ hasIpnSecret ? t('nowpayments.ipnSecretConfiguredHint') : t('nowpayments.ipnSecretHint') }}
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
              class="btn btn-secondary whitespace-nowrap text-xs flex items-center gap-1.5"
            >
              <svg v-if="copied" class="h-3.5 w-3.5 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>
              <span>{{ copied ? t('common.copied') : t('common.copy') }}</span>
            </button>
          </div>
        </div>

        <!-- Footer Actions -->
        <div class="p-6 flex items-center justify-end gap-3 bg-gray-50/30 dark:bg-dark-800/20">
          <button
            type="submit"
            :disabled="saving"
            class="btn btn-primary min-w-[100px]"
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

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const loaded = ref(false)
const saving = ref(false)
const copied = ref(false)

const showApiKey = ref(false)
const showIpnSecret = ref(false)

const providerId = ref<number | null>(null)
const hasApiKey = ref(false)
const hasIpnSecret = ref(false)

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
      form.sandbox = np.config?.sandbox === 'true' || np.config?.sandbox === '1'
      form.currency = np.config?.currency || 'USD'
      // Backend omits secrets in responses for security.
      // If the provider exists in DB, keys are already securely stored.
      hasApiKey.value = true
      hasIpnSecret.value = true
      form.apiKey = ''
      form.ipnSecret = ''
    } else {
      providerId.value = null
      form.enabled = false
      form.sandbox = true
      form.apiKey = ''
      form.ipnSecret = ''
      form.currency = 'USD'
      hasApiKey.value = false
      hasIpnSecret.value = false
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
    // Clipboard permission fallback
  }
}

async function saveSettings() {
  // If first-time creation, both keys are required
  if (!providerId.value && (!form.apiKey.trim() || !form.ipnSecret.trim())) {
    appStore.showError(t('nowpayments.keysRequired'))
    return
  }

  saving.value = true
  try {
    const configData: Record<string, string> = {
      sandbox: form.sandbox ? 'true' : 'false',
      currency: (form.currency.trim() || 'USD').toUpperCase(),
      notifyUrl: webhookUrl.value,
    }

    // Only send keys if entered; empty values on existing provider will preserve secrets
    if (form.apiKey.trim()) {
      configData.apiKey = form.apiKey.trim()
    } else if (providerId.value) {
      configData.apiKey = ''
    }

    if (form.ipnSecret.trim()) {
      configData.ipnSecret = form.ipnSecret.trim()
    } else if (providerId.value) {
      configData.ipnSecret = ''
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
