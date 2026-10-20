<template>
  <AppLayout>
    <div class="p-4 sm:p-6">
      <!-- Header -->
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('modelsCatalog.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('modelsCatalog.description') }}</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <div class="relative w-full sm:w-72">
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input v-model="search" type="text" class="input pl-10" :placeholder="t('modelsCatalog.searchPlaceholder')" />
          </div>
          <div class="flex overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
            <button
              v-for="view in views"
              :key="view.id"
              type="button"
              class="px-3 py-2 text-xs font-medium transition-colors"
              :class="activeView === view.id
                ? 'bg-primary-600 text-white'
                : 'bg-white text-gray-600 hover:bg-gray-50 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700'"
              @click="activeView = view.id"
            >
              {{ t(view.labelKey) }}
            </button>
          </div>
        </div>
      </div>

      <!-- Filters -->
      <div class="mt-5 space-y-3">
        <div class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            class="rounded-full px-3 py-1 text-xs font-medium transition-colors"
            :class="activeType === 'all' ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300'"
            @click="activeType = 'all'"
          >
            {{ t('modelsCatalog.typeAll') }}
          </button>
          <button
            v-for="type in modelTypes"
            :key="type.id"
            type="button"
            class="rounded-full px-3 py-1 text-xs font-medium transition-colors"
            :class="activeType === type.id ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300'"
            @click="activeType = type.id"
          >
            {{ t(type.labelKey) }}
          </button>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            class="rounded-full px-3 py-1 text-xs font-medium transition-colors"
            :class="activeVendor === '' ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900' : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300'"
            @click="activeVendor = ''"
          >
            {{ t('modelsCatalog.allVendors') }}
          </button>
          <button
            v-for="vendor in vendors"
            :key="vendor"
            type="button"
            class="rounded-full px-3 py-1 text-xs font-medium capitalize transition-colors"
            :class="activeVendor === vendor ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900' : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300'"
            @click="activeVendor = activeVendor === vendor ? '' : vendor"
          >
            {{ vendor }} ({{ vendorCounts[vendor] || 0 }})
          </button>
        </div>
      </div>

      <div class="mt-4 flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
        <span>{{ t('modelsCatalog.modelsCount', { count: filtered.length }) }}</span>
        <span v-if="pricedCount !== filtered.length">{{ t('modelsCatalog.unpricedNote', { count: filtered.length - pricedCount }) }}</span>
      </div>

      <!-- Loading -->
      <div v-if="loading" class="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
        <div v-for="n in 6" :key="n" class="h-28 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-800"></div>
      </div>

      <!-- Empty -->
      <div v-else-if="filtered.length === 0" class="mt-10 text-center text-sm text-gray-500 dark:text-gray-400">
        {{ t('modelsCatalog.noResults') }}
      </div>

      <!-- Catalog cards -->
      <div v-else-if="activeView === 'catalog'" class="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
        <div
          v-for="model in filtered"
          :key="model.canonical_model_id || model.model_id"
          class="flex flex-col justify-between rounded-xl border border-black/[0.07] bg-gradient-to-b from-white to-[#f7f7f7] p-4 shadow-[0_0_20px_rgba(0,0,0,0.04)] dark:border-dark-700 dark:from-dark-800 dark:to-dark-800"
        >
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <p class="truncate text-sm font-medium text-gray-900 dark:text-white" :title="model.model_id">
                {{ shortName(model) }}
              </p>
              <p class="mt-0.5 truncate text-[11px] text-gray-400" :title="model.canonical_model_id">
                {{ model.canonical_model_id || model.model_id }}
              </p>
            </div>
            <span
              v-if="isAvailable(model)"
              class="mt-1 inline-block h-2 w-2 shrink-0 rounded-full bg-emerald-500"
              :title="t('modelsCatalog.available')"
            ></span>
            <span
              v-else
              class="mt-1 inline-block h-2 w-2 shrink-0 rounded-full bg-gray-300"
              :title="t('modelsCatalog.unavailable')"
            ></span>
          </div>

          <div class="mt-3 flex items-end justify-between gap-2">
            <div class="text-xs text-gray-600 dark:text-gray-300">
              <template v-if="model.input_price_per_1m != null || model.output_price_per_1m != null">
                <span class="text-gray-400">{{ t('modelsCatalog.input') }}</span>
                <span class="ml-1 font-medium">{{ price(model.input_price_per_1m) }}</span>
                <span class="mx-1.5 text-gray-300">·</span>
                <span class="text-gray-400">{{ t('modelsCatalog.output') }}</span>
                <span class="ml-1 font-medium">{{ price(model.output_price_per_1m) }}</span>
              </template>
              <template v-else-if="model.native_price_usd != null">
                <span class="font-medium">${{ formatNative(model.native_price_usd) }}</span>
                <span class="ml-1 text-gray-400">/ {{ model.native_price_unit || 'request' }}</span>
              </template>
              <span v-else class="text-gray-400">{{ t('modelsCatalog.noPrice') }}</span>
            </div>
            <span
              v-if="model.discount_percent && model.discount_percent > 0"
              class="shrink-0 rounded-md bg-emerald-50 px-2 py-0.5 text-[11px] font-semibold text-emerald-600 dark:bg-emerald-900/20 dark:text-emerald-400"
            >
              -{{ model.discount_percent }}%
            </span>
          </div>
        </div>
      </div>

      <!-- Names, grouped by vendor -->
      <div v-else class="mt-6 space-y-6">
        <div v-for="group in groupedByVendor" :key="group.vendor">
          <div class="flex items-center gap-2">
            <h3 class="text-sm font-semibold capitalize text-gray-900 dark:text-white">{{ group.vendor }}</h3>
            <span class="text-xs text-gray-400">({{ group.models.length }})</span>
          </div>
          <div class="mt-2 flex flex-wrap gap-1.5">
            <span
              v-for="model in group.models"
              :key="model.canonical_model_id || model.model_id"
              class="rounded-lg border border-gray-200 bg-white px-2 py-1 font-mono text-[11px] text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300"
              :title="model.canonical_model_id || model.model_id"
            >
              {{ shortName(model) }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { getPublicModels, type PublicModelPricing } from '@/api/publicModels'

const { t } = useI18n()

const models = ref<PublicModelPricing[]>([])
const loading = ref(true)
const search = ref('')
const activeType = ref<'all' | 'text' | 'image' | 'video' | 'audio' | 'embedding'>('all')
const activeVendor = ref('')
const activeView = ref<'catalog' | 'names'>('catalog')

const views = [
  { id: 'catalog' as const, labelKey: 'modelsCatalog.viewCatalog' },
  { id: 'names' as const, labelKey: 'modelsCatalog.viewNames' },
]

const modelTypes = [
  { id: 'text' as const, labelKey: 'modelsCatalog.typeText' },
  { id: 'image' as const, labelKey: 'modelsCatalog.typeImage' },
  { id: 'video' as const, labelKey: 'modelsCatalog.typeVideo' },
  { id: 'audio' as const, labelKey: 'modelsCatalog.typeAudio' },
  { id: 'embedding' as const, labelKey: 'modelsCatalog.typeEmbedding' },
]

/** Vendor = the canonical prefix (`anthropic/claude-…` -> `anthropic`). */
function vendorOf(model: PublicModelPricing): string {
  const id = model.canonical_model_id || model.model_id || ''
  const slash = id.indexOf('/')
  return slash > 0 ? id.slice(0, slash).toLowerCase() : 'other'
}

/** Model slug without the vendor prefix. */
function shortName(model: PublicModelPricing): string {
  const id = model.canonical_model_id || model.model_id || ''
  const slash = id.indexOf('/')
  return slash > 0 ? id.slice(slash + 1) : id
}

/** Coarse capability bucket inferred from the model id / billing unit. */
function typeOf(model: PublicModelPricing): 'text' | 'image' | 'video' | 'audio' | 'embedding' {
  const id = (model.model_id || model.canonical_model_id || '').toLowerCase()
  const unit = (model.native_price_unit || '').toLowerCase()
  if (id.includes('embed')) return 'embedding'
  if (/(tts|whisper|audio|speech|voice|music|lyria|suno|eleven)/.test(id) || unit === 'second' && id.includes('audio')) return 'audio'
  if (/(veo|sora|kling|runway|hailuo|video|wan2|luma|pika)/.test(id)) return 'video'
  if (/(image|imagen|dall-e|dalle|flux|stable-diffusion|midjourney|nano-banana|seedream|gpt-image)/.test(id)) return 'image'
  return 'text'
}

const isAvailable = (model: PublicModelPricing) => model.available !== false && model.status !== 'unavailable'

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  return models.value.filter((model) => {
    if (activeType.value !== 'all' && typeOf(model) !== activeType.value) return false
    if (activeVendor.value && vendorOf(model) !== activeVendor.value) return false
    if (!q) return true
    return (model.model_id || '').toLowerCase().includes(q) || (model.canonical_model_id || '').toLowerCase().includes(q)
  })
})

const pricedCount = computed(
  () => filtered.value.filter((m) => m.input_price_per_1m != null || m.output_price_per_1m != null || m.native_price_usd != null).length
)

const vendors = computed(() => {
  const counted = new Map<string, number>()
  for (const model of models.value) {
    const vendor = vendorOf(model)
    counted.set(vendor, (counted.get(vendor) || 0) + 1)
  }
  return [...counted.keys()].sort()
})

const vendorCounts = computed(() => {
  const counted: Record<string, number> = {}
  for (const model of models.value) {
    const vendor = vendorOf(model)
    counted[vendor] = (counted[vendor] || 0) + 1
  }
  return counted
})

const groupedByVendor = computed(() => {
  const groups = new Map<string, PublicModelPricing[]>()
  for (const model of filtered.value) {
    const vendor = vendorOf(model)
    if (!groups.has(vendor)) groups.set(vendor, [])
    groups.get(vendor)!.push(model)
  }
  return [...groups.entries()]
    .sort((a, b) => b[1].length - a[1].length)
    .map(([vendor, list]) => ({
      vendor,
      models: list.slice().sort((a, b) => shortName(a).localeCompare(shortName(b)))
    }))
})

/** Retail price per 1M tokens, or an em dash when the model has no token price. */
function price(value: number | null | undefined): string {
  if (value == null) return '—'
  if (value >= 1) return `$${value.toFixed(2)}/1M`
  return `$${value.toFixed(4)}/1M`
}

function formatNative(value: number): string {
  return value >= 1 ? value.toFixed(2) : value.toFixed(4)
}

async function load() {
  loading.value = true
  try {
    models.value = await getPublicModels()
  } catch {
    models.value = []
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>