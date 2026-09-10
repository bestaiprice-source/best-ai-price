<template>
  <AppLayout>
    <div class="space-y-4">
      <PaymentProviderList
        :providers="providers"
        :loading="loading"
        :can-create="true"
        :enabled-payment-types="enabledPaymentTypes"
        :all-payment-types="allPaymentTypes"
        :redirect-label="redirectLabel"
        @refresh="loadProviders"
        @create="openCreate"
        @edit="openEdit"
        @delete="confirmDelete"
        @toggle-field="handleToggleField"
        @toggle-type="handleToggleType"
        @reorder="handleReorder"
      />
    </div>

    <!-- Create / Edit Dialog -->
    <PaymentProviderDialog
      :show="showDialog"
      :saving="saving"
      :editing="editingProvider"
      :all-key-options="allKeyOptions"
      :enabled-key-options="enabledKeyOptions"
      :all-payment-types="allPaymentTypes"
      :redirect-label="redirectLabel"
      @close="showDialog = false"
      @save="handleSave"
    />

    <!-- Delete Confirmation -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.settings.payment.deleteProvider')"
      :message="t('admin.settings.payment.deleteProviderConfirm')"
      :confirm-text="t('common.delete')"
      danger
      @confirm="handleDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import type { ProviderInstance } from '@/types/payment'
import type { TypeOption } from '@/components/payment/providerConfig'
import AppLayout from '@/components/layout/AppLayout.vue'
import PaymentProviderList from '@/components/payment/PaymentProviderList.vue'
import PaymentProviderDialog from '@/components/payment/PaymentProviderDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const saving = ref(false)
const providers = ref<ProviderInstance[]>([])

const showDialog = ref(false)
const editingProvider = ref<ProviderInstance | null>(null)

const showDeleteDialog = ref(false)
const deletingProvider = ref<ProviderInstance | null>(null)

const allKeyOptions = computed<TypeOption[]>(() => [
  { value: 'nowpayments', label: t('admin.settings.payment.providerNowpayments') },
  { value: 'stripe', label: t('admin.settings.payment.providerStripe') },
  { value: 'easypay', label: t('admin.settings.payment.providerEasypay') },
  { value: 'alipay', label: t('admin.settings.payment.providerAlipay') },
  { value: 'wxpay', label: t('admin.settings.payment.providerWxpay') },
])

const enabledKeyOptions = computed(() => allKeyOptions.value)

const allPaymentTypes = computed<TypeOption[]>(() => [
  { value: 'nowpayments', label: t('payment.methods.nowpayments', 'Crypto (NOWPayments)') },
  { value: 'card', label: t('payment.methods.card', 'Credit Card') },
  { value: 'link', label: t('payment.methods.link', 'Link') },
  { value: 'alipay', label: t('payment.methods.alipay', 'Alipay') },
  { value: 'wxpay', label: t('payment.methods.wxpay', 'WeChat Pay') },
])

const redirectLabel = computed(() => t('payment.modes.redirect', 'Redirect'))

const enabledPaymentTypes = computed(() => [
  'nowpayments',
  'stripe',
  'easypay',
  'alipay',
  'wxpay',
])

async function loadProviders() {
  loading.value = true
  try {
    const res = await adminPaymentAPI.getProviders()
    providers.value = Array.isArray(res.data) ? res.data : []
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingProvider.value = null
  showDialog.value = true
}

function openEdit(provider: ProviderInstance) {
  editingProvider.value = provider
  showDialog.value = true
}

function confirmDelete(provider: ProviderInstance) {
  deletingProvider.value = provider
  showDeleteDialog.value = true
}

async function handleDelete() {
  if (!deletingProvider.value) return
  try {
    await adminPaymentAPI.deleteProvider(deletingProvider.value.id)
    appStore.showSuccess(t('common.deleted'))
    showDeleteDialog.value = false
    deletingProvider.value = null
    await loadProviders()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

async function handleToggleField(
  provider: ProviderInstance,
  field: 'enabled' | 'refund_enabled' | 'allow_user_refund'
) {
  try {
    const newVal = !provider[field]
    await adminPaymentAPI.updateProvider(provider.id, { [field]: newVal })
    provider[field] = newVal
    appStore.showSuccess(t('common.saved'))
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

async function handleToggleType(provider: ProviderInstance, type: string) {
  try {
    const nextTypes = provider.supported_types.includes(type)
      ? provider.supported_types.filter(t => t !== type)
      : [...provider.supported_types, type]
    await adminPaymentAPI.updateProvider(provider.id, { supported_types: nextTypes })
    provider.supported_types = nextTypes
    appStore.showSuccess(t('common.saved'))
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

async function handleReorder(updates: { id: number; sort_order: number }[]) {
  try {
    await Promise.all(
      updates.map(u => adminPaymentAPI.updateProvider(u.id, { sort_order: u.sort_order }))
    )
    appStore.showSuccess(t('common.saved'))
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

async function handleSave(payload: {
  provider_key: string
  name: string
  supported_types: string[]
  enabled: boolean
  payment_mode: string
  refund_enabled: boolean
  allow_user_refund: boolean
  config: Record<string, string>
  limits: string
}) {
  saving.value = true
  try {
    if (editingProvider.value) {
      await adminPaymentAPI.updateProvider(editingProvider.value.id, payload)
      appStore.showSuccess(t('common.saved'))
    } else {
      await adminPaymentAPI.createProvider(payload)
      appStore.showSuccess(t('common.created'))
    }
    showDialog.value = false
    await loadProviders()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  loadProviders()
})
</script>
