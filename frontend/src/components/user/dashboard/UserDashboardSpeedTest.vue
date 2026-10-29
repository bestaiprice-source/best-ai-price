<template>
  <div class="bap-card p-5 sm:p-6">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0">
        <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('speedTest.title') }}</h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('speedTest.subtitle') }}</p>
      </div>
      <div v-if="result" class="text-right">
        <p class="font-mono text-2xl font-bold tracking-tight text-[#0355cd] dark:text-[#60a5fa]">
          {{ result.ttft_ms }} ms
        </p>
        <p class="text-[11px] text-gray-400">{{ t('speedTest.ttft') }}</p>
      </div>
    </div>

    <div class="mt-4 flex flex-col gap-2 sm:flex-row sm:items-center">
      <select v-model="model" class="input sm:max-w-xs" :aria-label="t('speedTest.model')">
        <option v-if="!models.length" value="">{{ t('speedTest.loadingModels') }}</option>
        <option v-for="option in models" :key="option" :value="option">{{ option }}</option>
      </select>
      <button
        type="button"
        class="bap-btn-accent justify-center whitespace-nowrap disabled:opacity-60"
        :disabled="running || !model"
        @click="run"
      >
        <span v-if="running">{{ t('speedTest.running') }}</span>
        <span v-else>{{ t('speedTest.run') }}</span>
      </button>
    </div>

    <div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-gray-500 dark:text-dark-400">
      <template v-if="result">
        <span>
          {{ t('speedTest.total') }}:
          <span class="font-mono font-medium text-gray-900 dark:text-white">{{ result.total_ms }} ms</span>
        </span>
        <span>
          {{ t('speedTest.tokens') }}:
          <span class="font-mono font-medium text-gray-900 dark:text-white">
            {{ result.input_tokens }} / {{ result.output_tokens }}
          </span>
        </span>
        <span v-if="result.node_id" :title="result.node_id">
          {{ t('speedTest.servedBy') }}:
          <span class="font-mono">{{ shortNode }}</span>
        </span>
        <span v-if="measuredAt" class="text-gray-400">{{ measuredAt }}</span>
      </template>
      <span v-else-if="!running">{{ t('speedTest.idle') }}</span>
    </div>

    <p v-if="error" class="mt-3 rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-700 dark:bg-rose-900/20 dark:text-rose-300">
      {{ error }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { runSpeedTest, type SpeedTestResult } from '@/api/speedTest'
import { getPublicModels } from '@/api/publicModels'
import { extractApiErrorMessage } from '@/utils/apiError'

/**
 * "API speed" card: the measured latency of a real minimal completion plus a
 * button to re-measure it.
 *
 * The model list is the priced catalog (the same feed the gateway bills from),
 * so the button can never be pointed at a model the gateway would refuse. The
 * request is a REAL billed completion — max_tokens 1 — which is what makes the
 * number meaningful; the last measurement is kept in localStorage so the card
 * shows something after a reload instead of resetting to an empty state.
 */
const STORAGE_KEY = 'sub2api_speed_test'

const { t } = useI18n()

const models = ref<string[]>([])
const model = ref('')
const running = ref(false)
const error = ref('')
const result = ref<SpeedTestResult | null>(null)

const shortNode = computed(() => (result.value?.node_id ?? '').slice(0, 12))

const measuredAt = computed(() => {
  if (!storedAt.value) return ''
  return new Date(storedAt.value).toLocaleTimeString()
})
const storedAt = ref<number>(0)

function restore() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return
    const parsed = JSON.parse(raw) as { result?: SpeedTestResult; at?: number }
    if (parsed?.result) {
      result.value = parsed.result
      storedAt.value = parsed.at ?? 0
      if (parsed.result.model) model.value = parsed.result.model
    }
  } catch {
    // A corrupt entry must not break the dashboard — ignore it.
  }
}

function persist(next: SpeedTestResult) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ result: next, at: Date.now() }))
  } catch {
    // Storage may be unavailable (private mode); the result is still shown.
  }
}

async function loadModels() {
  try {
    const all = await getPublicModels()
    const priced = all
      .filter((m) => m.input_price_per_1m != null || m.output_price_per_1m != null || m.native_price_usd != null)
      .map((m) => m.model_id || m.canonical_model_id || '')
      .filter(Boolean)
    models.value = [...new Set(priced)].sort((a, b) => a.localeCompare(b))
    if (!model.value && models.value.length) model.value = models.value[0]
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('speedTest.modelsFailed'))
  }
}

async function run() {
  if (running.value || !model.value) return
  running.value = true
  error.value = ''
  try {
    const next = await runSpeedTest(model.value)
    result.value = next
    storedAt.value = Date.now()
    persist(next)
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('speedTest.failed'))
  } finally {
    running.value = false
  }
}

onMounted(() => {
  restore()
  void loadModels()
})
</script>
