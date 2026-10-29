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
              class="px-3 py-2 text-sm font-medium transition-colors"
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
            class="rounded-full px-3.5 py-1.5 text-sm font-medium transition-colors"
            :class="activeType === 'all' ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300'"
            @click="activeType = 'all'"
          >
            {{ t('modelsCatalog.typeAll') }}
          </button>
          <button
            v-for="type in modelTypes"
            :key="type.id"
            type="button"
            class="rounded-full px-3.5 py-1.5 text-sm font-medium transition-colors"
            :class="activeType === type.id ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300'"
            @click="activeType = type.id"
          >
            {{ t(type.labelKey) }}
          </button>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            class="rounded-full px-3.5 py-1.5 text-sm font-medium transition-colors"
            :class="activeVendor === '' ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900' : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300'"
            @click="activeVendor = ''"
          >
            {{ t('modelsCatalog.allVendors') }}
          </button>
          <button
            v-for="vendor in vendors"
            :key="vendor"
            type="button"
            class="rounded-full px-3.5 py-1.5 text-sm font-medium capitalize transition-colors"
            :class="activeVendor === vendor ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900' : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300'"
            @click="activeVendor = activeVendor === vendor ? '' : vendor"
          >
            {{ vendor }} ({{ vendorCounts[vendor] || 0 }})
          </button>
        </div>
      </div>

      <ModelHealthLegend v-if="hasHealth" class="mt-4" />

      <div class="mt-4 text-xs text-gray-500 dark:text-gray-400">
        <span>{{ t('modelsCatalog.modelsCount', { count: filtered.length }) }}</span>
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
        <router-link
          v-for="model in filtered"
          :key="model.canonical_model_id || model.model_id"
          to="/keys"
          class="model-card group flex flex-col justify-between rounded-xl border border-black/[0.07] bg-gradient-to-b from-white to-[#f7f7f7] p-4 shadow-[0_0_20px_rgba(0,0,0,0.04)] transition-all hover:-translate-y-0.5 hover:border-black/20 hover:shadow-[0_8px_25px_rgba(0,0,0,0.07)] dark:border-dark-700 dark:from-dark-800 dark:to-dark-800 dark:hover:border-white/20"
          :title="t('modelsCatalog.useKeyTitle')"
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
            <div class="flex shrink-0 items-center gap-2">
              <span
                v-if="discountOf(model)"
                class="rounded-full bg-emerald-50 px-2 py-0.5 text-[11px] font-semibold text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300"
                :title="t('modelsCatalog.discountTitle', { percent: discountOf(model) })"
              >
                -{{ discountOf(model) }}%
              </span>
              <button
                type="button"
                class="rounded-md p-1 text-gray-400 transition-colors hover:bg-black/5 hover:text-gray-700 dark:hover:bg-white/10 dark:hover:text-white"
                :title="t('common.copy')"
                :aria-label="t('common.copy')"
                @click.prevent.stop="copyModelName(model)"
              >
                <Icon name="copy" size="sm" />
              </button>
              <!-- Probe health when a monitor covers this model, otherwise the
                   catalog's own binary availability flag. -->
              <ModelHealthBadge
                v-if="healthOf(model)"
                :status="healthOf(model)?.status"
                :uptime="healthOf(model)?.availability_pct ?? null"
              />
              <span
                v-else-if="isAvailable(model)"
                class="inline-block h-2 w-2 shrink-0 rounded-full bg-emerald-500"
                :title="t('modelsCatalog.available')"
              ></span>
              <span
                v-else
                class="inline-block h-2 w-2 shrink-0 rounded-full bg-gray-300"
                :title="t('modelsCatalog.unavailable')"
              ></span>
            </div>
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
            <!-- Every card leads to the API keys page: get a key and call this model. -->
            <span class="inline-flex shrink-0 items-center gap-1 text-[11px] font-medium text-gray-400 transition-colors group-hover:text-[#4791ff] dark:text-dark-400">
              {{ t('modelsCatalog.useKey') }}
              <span aria-hidden="true">&rarr;</span>
            </span>
          </div>
        </router-link>
      </div>

      <!-- Names, grouped by vendor -->
      <div v-else class="mt-6 space-y-6">
        <div v-for="group in groupedByVendor" :key="group.vendor">
          <div class="flex items-center gap-2">
            <h3 class="text-sm font-semibold capitalize text-gray-900 dark:text-white">{{ group.vendor }}</h3>
            <span class="text-xs text-gray-400">({{ group.models.length }})</span>
          </div>
          <div class="mt-2 flex flex-wrap gap-1.5">
            <button
              v-for="model in group.models"
              :key="model.canonical_model_id || model.model_id"
              type="button"
              class="rounded-lg border border-gray-200 bg-white px-2 py-1 font-mono text-[11px] text-gray-700 transition-colors hover:border-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300"
              :title="t('common.copy')"
              @click="copyModelName(model)"
            >
              {{ shortName(model) }}
            </button>
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
import ModelHealthBadge from '@/components/user/monitor/ModelHealthBadge.vue'
import ModelHealthLegend from '@/components/user/monitor/ModelHealthLegend.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useModelHealth } from '@/composables/useModelHealth'
import { getPublicModels, type PublicModelPricing } from '@/api/publicModels'

const { t } = useI18n()
const { items: healthItems, load: loadHealth, resolve: resolveHealth } = useModelHealth()

/**
 * Probe-measured health for a catalog model.
 *
 * The catalog feed only carries a binary availability flag, so the uptime
 * percentage and the Normal / High latency / Error / Maintenance status come
 * from the channel monitors, joined on the gateway's own normalization keys
 * (see useModelHealth). A model no monitor covers returns null and the card
 * falls back to the catalog's own availability dot.
 */
const healthOf = (model: PublicModelPricing) =>
  resolveHealth(model.canonical_model_id || model.model_id)

const hasHealth = computed(() => healthItems.value.length > 0)

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

const { copyToClipboard } = useClipboard()

/**
 * How far below the vendor's official list price we sell, as a whole percent.
 * Comes from the public catalog feed (same retail basis the gateway charges), so
 * it moves together with provider price changes; hidden when there is no saving.
 */
function discountOf(model: PublicModelPricing): number {
  const value = Math.round(Number(model.discount_percent ?? 0))
  return value >= 1 && value <= 99 ? value : 0
}

/** Copies the model id exactly as it must be sent in the API `model` field. */
function copyModelName(model: PublicModelPricing) {
  void copyToClipboard(model.model_id || model.canonical_model_id || '', t('common.copiedToClipboard'))
}

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

/** A model is listed only when we can show what it costs — token or per-unit price. */
function hasPrice(model: PublicModelPricing): boolean {
  return (
    model.input_price_per_1m != null ||
    model.output_price_per_1m != null ||
    model.native_price_usd != null
  )
}

/** Models without any published price are hidden from the whole page. */
const pricedModels = computed(() => models.value.filter(hasPrice))

/**
 * Models matching the active type + search but NOT the vendor chip. The chip row
 * is built from this so every counter follows the filter the user just applied —
 * selecting "Video" must not keep showing the all-types counts. The vendor
 * filter itself is excluded, otherwise picking one provider would zero out every
 * other chip and there would be no way back.
 */
const facetModels = computed(() => {
  const q = search.value.trim().toLowerCase()
  return pricedModels.value.filter((model) => {
    if (activeType.value !== 'all' && typeOf(model) !== activeType.value) return false
    if (!q) return true
    return (model.model_id || '').toLowerCase().includes(q) || (model.canonical_model_id || '').toLowerCase().includes(q)
  })
})

/** Biggest discount first; equal discounts keep the feed order (stable sort). */
function byDiscountDesc(a: PublicModelPricing, b: PublicModelPricing): number {
  return (Number(b.discount_percent) || 0) - (Number(a.discount_percent) || 0)
}

const filtered = computed(() =>
  (activeVendor.value
    ? facetModels.value.filter((model) => vendorOf(model) === activeVendor.value)
    : facetModels.value
  )
    .slice()
    .sort(byDiscountDesc)
)

/** Per-vendor model counts for the CURRENT type + search. */
const vendorCounts = computed(() => {
  const counted: Record<string, number> = {}
  for (const model of facetModels.value) {
    const vendor = vendorOf(model)
    counted[vendor] = (counted[vendor] || 0) + 1
  }
  return counted
})

/**
 * Provider chips: the three flagship vendors come first in a fixed order
 * (anthropic / Claude, openai, google / Gemini), then every other vendor by
 * popularity. A vendor's popularity here is how many models it offers under the
 * current filter — the same number the chip shows — with alphabetical
 * tie-breaks. The `other` bucket (models carrying no vendor
 * prefix) is not a provider, so it stays pinned at the end instead of leading
 * the list.
 *
 * A provider left with no model under the current filter is not offered at all;
 * the currently selected one stays so it can still be deselected.
 *
 * The click-based provider popularity (`GET /providers/popularity`, the
 * `provider_popularity` table) is keyed by UPSTREAM provider names
 * (openlux / portdan / …), which never match these model vendor prefixes, so it
 * cannot order this row.
 */
const PINNED_VENDORS = ['anthropic', 'openai', 'google']

const vendors = computed(() => {
  const counts = vendorCounts.value
  const visible = Object.keys(counts).filter((vendor) => (counts[vendor] ?? 0) > 0)
  // Keep the selected chip even when the current filter leaves it with nothing,
  // otherwise the only way back to "all providers" is the leading chip.
  if (activeVendor.value && !visible.includes(activeVendor.value)) {
    visible.push(activeVendor.value)
  }
  return visible.sort((aVendor, bVendor) => {
    if (aVendor === 'other') return 1
    if (bVendor === 'other') return -1
    const aPin = PINNED_VENDORS.indexOf(aVendor)
    const bPin = PINNED_VENDORS.indexOf(bVendor)
    if (aPin !== -1 || bPin !== -1) {
      if (aPin === -1) return 1
      if (bPin === -1) return -1
      return aPin - bPin
    }
    const diff = (counts[bVendor] ?? 0) - (counts[aVendor] ?? 0)
    return diff !== 0 ? diff : aVendor.localeCompare(bVendor)
  })
})

const groupedByVendor = computed(() => {
  const groups = new Map<string, PublicModelPricing[]>()
  for (const model of filtered.value) {
    const vendor = vendorOf(model)
    if (!groups.has(vendor)) groups.set(vendor, [])
    groups.get(vendor)!.push(model)
  }
  return [...groups.entries()]
    .sort((a, b) => {
      if (a[0] === 'other') return 1
      if (b[0] === 'other') return -1
      const aPin = PINNED_VENDORS.indexOf(a[0])
      const bPin = PINNED_VENDORS.indexOf(b[0])
      if (aPin !== -1 || bPin !== -1) {
        if (aPin === -1) return 1
        if (bPin === -1) return -1
        return aPin - bPin
      }
      return b[1].length - a[1].length
    })
    .map(([vendor, list]) => ({
      vendor,
      models: list.slice().sort((a, b) => byDiscountDesc(a, b) || shortName(a).localeCompare(shortName(b)))
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
// Health shares one request with the dashboard (module-level state) and never
// blocks the catalog: a failure just leaves the cards on the binary dot.
onMounted(() => {
  void loadHealth()
})
</script>