<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Header -->
      <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
            {{ t('billing.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('billing.description') }}
          </p>
        </div>
        <button
          class="inline-flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 shadow-sm transition-all hover:bg-gray-50 disabled:opacity-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200"
          :disabled="loading"
          @click="load"
        >
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
          <span>{{ loading ? t('common.loading') : t('common.refresh') }}</span>
        </button>
      </div>

      <!-- Totals -->
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('billing.types.deposit') }}</p>
          <p class="mt-2 text-2xl font-bold text-emerald-600">${{ formatAmount(ledger?.total_deposit_usd || 0) }}</p>
        </div>
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('billing.types.usage') }}</p>
          <p class="mt-2 text-2xl font-bold text-rose-600">${{ formatAmount(ledger?.total_usage_usd || 0) }}</p>
        </div>
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('billing.types.refund') }}</p>
          <p class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">${{ formatAmount(ledger?.total_refund_usd || 0) }}</p>
        </div>
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('billing.types.adjustment') }}</p>
          <p class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">${{ formatAmount(ledger?.total_adjustment_usd || 0) }}</p>
        </div>
      </div>

      <!-- Type filter -->
      <div class="flex flex-wrap gap-2">
        <button
          v-for="opt in typeOptions"
          :key="opt.value"
          type="button"
          class="rounded-full border px-3 py-1 text-xs font-medium transition-colors"
          :class="typeFilter === opt.value
            ? 'border-primary-400 bg-primary-50 text-primary-700 dark:border-primary-700 dark:bg-primary-950/40 dark:text-primary-300'
            : 'border-gray-200 bg-white text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300'"
          @click="setType(opt.value)"
        >
          {{ opt.label }}
        </button>
      </div>

      <!-- Table -->
      <div class="overflow-hidden rounded-2xl border border-gray-200/80 bg-white/85 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50/60 text-left text-xs uppercase tracking-wider text-gray-500 dark:bg-dark-900/40 dark:text-gray-400">
              <tr>
                <th class="px-4 py-3">{{ t('billing.columns.time') }}</th>
                <th class="px-4 py-3">{{ t('billing.columns.type') }}</th>
                <th class="px-4 py-3 text-right">{{ t('billing.columns.amount') }}</th>
                <th class="px-4 py-3">{{ t('billing.columns.asset') }}</th>
                <th class="px-4 py-3">{{ t('billing.columns.status') }}</th>
                <th class="px-4 py-3">{{ t('billing.columns.details') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-50 dark:divide-dark-800">
              <tr v-for="row in rows" :key="row.id">
                <td class="whitespace-nowrap px-4 py-3 text-gray-500 dark:text-gray-400">{{ formatTime(row.created_at) }}</td>
                <td class="px-4 py-3"><span :class="typeBadgeClass(row.type)">{{ t('billing.types.' + row.type) }}</span></td>
                <td class="px-4 py-3 text-right font-mono" :class="row.amount_usd >= 0 ? 'text-emerald-600' : 'text-rose-600'">
                  {{ row.amount_usd >= 0 ? '+' : '-' }}${{ formatAmount(Math.abs(row.amount_usd)) }}
                </td>
                <td class="px-4 py-3 text-gray-600 dark:text-gray-300">
                  <span>{{ row.currency || 'USD' }}</span>
                  <span v-if="row.network" class="ml-1 text-xs text-gray-400">{{ row.network }}</span>
                </td>
                <td class="px-4 py-3 text-xs uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ row.status }}</td>
                <td class="px-4 py-3">
                  <div class="text-gray-800 dark:text-gray-200">{{ row.description }}</div>
                  <div v-if="row.reference" class="font-mono text-xs text-gray-400">{{ row.reference }}</div>
                </td>
              </tr>
              <tr v-if="!loading && rows.length === 0">
                <td colspan="6" class="px-4 py-10 text-center text-gray-400">{{ t('billing.empty') }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { getLedger, type LedgerEntry, type BalanceLedgerPage } from '@/api/billing'

const { t } = useI18n()
const appStore = useAppStore()

const ledger = ref<BalanceLedgerPage | null>(null)
const loading = ref(false)
const typeFilter = ref('all')
const pagination = ref({ page: 1, page_size: 20, total: 0 })

const rows = computed<LedgerEntry[]>(() => ledger.value?.items || [])

const typeOptions = computed(() => [
  { value: 'all', label: t('billing.filterAll') },
  { value: 'deposit', label: t('billing.types.deposit') },
  { value: 'usage', label: t('billing.types.usage') },
  { value: 'refund', label: t('billing.types.refund') },
  { value: 'adjustment', label: t('billing.types.adjustment') },
])

const formatAmount = (v: number) => Number(v || 0).toFixed(4)
const formatTime = (v?: string | null) => (v ? new Date(v).toLocaleString() : '—')

const typeBadgeClass = (type: string) => {
  const base = 'inline-flex rounded-full px-2 py-0.5 text-xs font-medium '
  switch (type) {
    case 'deposit':
      return base + 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
    case 'refund':
      return base + 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
    case 'adjustment':
      return base + 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
    default:
      return base + 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-400'
  }
}

async function load() {
  loading.value = true
  try {
    ledger.value = await getLedger({
      type: typeFilter.value === 'all' ? undefined : typeFilter.value,
      page: pagination.value.page,
      page_size: pagination.value.page_size,
    })
    pagination.value.total = ledger.value?.total || 0
  } catch (e) {
    console.warn('Failed to load billing ledger:', e)
    appStore.showError(t('billing.loadFailed'))
  } finally {
    loading.value = false
  }
}

function setType(value: string) {
  typeFilter.value = value
  pagination.value.page = 1
  load()
}

function handlePageChange(page: number) {
  pagination.value.page = page
  load()
}

function handlePageSizeChange(size: number) {
  pagination.value.page_size = size
  pagination.value.page = 1
  load()
}

onMounted(load)
</script>
