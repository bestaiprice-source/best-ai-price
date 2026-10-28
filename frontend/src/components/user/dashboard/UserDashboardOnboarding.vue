<template>
  <section class="bap-card p-6 sm:p-7">
    <!-- Header + progress -->
    <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div class="space-y-1">
        <h2 class="text-base font-bold tracking-tight text-gray-900 sm:text-lg dark:text-white">
          {{ t('dashboard.onboarding.title') }}
        </h2>
        <p class="max-w-2xl text-xs text-gray-500 sm:text-sm dark:text-gray-400">
          {{ t('dashboard.onboarding.subtitle') }}
        </p>
      </div>
      <span class="bap-pill bap-pill-blue shrink-0 font-semibold">
        {{ t('dashboard.onboarding.progress', { done: doneCount, total: steps.length }) }}
      </span>
    </div>

    <div class="mt-4 h-1.5 w-full overflow-hidden rounded-full bg-black/[0.06] dark:bg-white/10">
      <div
        class="h-full rounded-full bg-[#4791ff] transition-all duration-300"
        :style="{ width: `${progressPercent}%` }"
      />
    </div>

    <!-- The three steps -->
    <ol class="mt-5 grid grid-cols-1 gap-3 lg:grid-cols-3">
      <li
        v-for="(step, index) in steps"
        :key="step.id"
        class="flex flex-col gap-3 rounded-2xl border p-4 transition-colors"
        :class="
          step.done
            ? 'border-emerald-200 bg-emerald-50/60 dark:border-emerald-900/50 dark:bg-emerald-950/20'
            : 'border-black/[0.07] bg-white dark:border-white/[0.08] dark:bg-dark-800'
        "
      >
        <div class="flex items-center gap-3">
          <span
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-bold"
            :class="
              step.done
                ? 'bg-emerald-500 text-white'
                : 'bg-black/[0.05] text-gray-500 dark:bg-white/10 dark:text-gray-300'
            "
          >
            <Icon v-if="step.done" name="check" size="sm" :stroke-width="3" />
            <template v-else>{{ index + 1 }}</template>
          </span>
          <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ step.title }}</p>
        </div>

        <p class="flex-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">
          {{ step.description }}
        </p>

        <router-link
          v-if="step.to && !step.done"
          :to="step.to"
          class="inline-flex w-fit items-center gap-1.5 rounded-xl bg-[#4791ff] px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-[#327df0]"
        >
          {{ step.action }}
          <Icon name="chevronRight" size="xs" />
        </router-link>
        <span
          v-else-if="step.done"
          class="inline-flex w-fit items-center gap-1.5 text-xs font-semibold text-emerald-600 dark:text-emerald-400"
        >
          <Icon name="checkCircle" size="sm" />
          {{ t('dashboard.onboarding.done') }}
        </span>
      </li>
    </ol>

    <!-- First API request -->
    <div class="mt-6 rounded-2xl border border-black/[0.07] bg-white p-4 dark:border-white/[0.08] dark:bg-[#161b22]">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex items-center gap-2">
          <Icon name="terminal" size="sm" class="text-[#4791ff]" />
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('dashboard.onboarding.firstRequest') }}
          </h3>
        </div>
        <div class="flex items-center gap-3">
          <button
            v-if="revealableKeyId && !insertedKey"
            type="button"
            class="text-[11px] font-semibold text-[#4791ff] hover:underline disabled:opacity-50"
            :disabled="revealing"
            @click="insertMyKey"
          >
            {{ revealing ? t('dashboard.onboarding.insertingKey') : t('dashboard.onboarding.useMyKey') }}
          </button>
          <span v-else-if="insertedKey" class="bap-pill bap-pill-green text-[10px]">
            {{ t('dashboard.onboarding.keyInserted') }}
          </span>
          <button
            type="button"
            class="inline-flex items-center gap-1 text-[11px] font-medium text-gray-500 transition-colors hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100"
            @click="copy(snippet, 'snippet')"
          >
            <Icon :name="copied === 'snippet' ? 'check' : 'clipboard'" size="sm" />
            {{ copied === 'snippet' ? t('dashboard.onboarding.copied') : t('dashboard.onboarding.copy') }}
          </button>
        </div>
      </div>

      <div class="mt-3 flex items-center gap-2 rounded-lg border border-black/[0.06] bg-black/[0.02] px-3 py-2 dark:border-white/[0.08] dark:bg-white/[0.03]">
        <span class="shrink-0 text-[10px] font-semibold uppercase tracking-wider text-gray-400">
          {{ t('dashboard.onboarding.baseUrl') }}
        </span>
        <code class="min-w-0 flex-1 truncate font-mono text-xs text-gray-700 dark:text-gray-300">{{ openAIBase }}</code>
        <button
          type="button"
          class="shrink-0 text-gray-400 transition-colors hover:text-gray-700 dark:hover:text-gray-200"
          :title="t('dashboard.onboarding.copy')"
          @click="copy(openAIBase, 'base')"
        >
          <Icon :name="copied === 'base' ? 'check' : 'clipboard'" size="sm" />
        </button>
      </div>

      <!-- Language tabs: the same example in Python / Node.js / cURL -->
      <div class="mt-3 flex gap-1 border-b border-black/[0.07] dark:border-white/[0.08]">
        <button
          v-for="lang in languages"
          :key="lang.id"
          type="button"
          class="rounded-t-lg px-3 py-1.5 text-xs font-medium transition-colors"
          :class="
            activeLanguage === lang.id
              ? 'bg-black/[0.05] text-gray-900 dark:bg-white/10 dark:text-white'
              : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100'
          "
          @click="activeLanguage = lang.id"
        >
          {{ lang.label }}
        </button>
      </div>

      <pre class="mt-2 max-h-64 overflow-auto rounded-lg bg-gray-900 p-3 text-[11px] leading-relaxed text-gray-100"><code>{{ snippet }}</code></pre>

      <p class="mt-2 text-[11px] text-gray-400">
        {{ t('dashboard.onboarding.firstRequestHint') }}
        <router-link to="/keys" class="font-medium text-[#4791ff] hover:underline">
          {{ t('dashboard.onboarding.openKeys') }}
        </router-link>
      </p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { useClipboard } from '@/composables/useClipboard'
import { keysAPI } from '@/api'
import { buildApiSnippet, type ApiSnippetLanguage } from '@/utils/apiSnippets'
import { getPublicApiBaseUrl } from '@/utils/url'
import type { ApiKey } from '@/types'

const props = defineProps<{
  /** The account already owns at least one API key. */
  hasKey: boolean
  /** The account balance is above zero. */
  hasBalance: boolean
  /** The account has served at least one request. */
  hasRequest: boolean
  /** The account's first key, used by the "use my key" action. */
  firstKey?: ApiKey | null
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const copied = ref('')
const revealing = ref(false)
const insertedKey = ref('')

/** Placeholder shown until the user asks for their own key to be inserted. */
const KEY_PLACEHOLDER = '<YOUR_API_KEY>'
const languages = [
  { id: 'python' as const, label: 'Python' },
  { id: 'nodejs' as const, label: 'Node.js' },
  { id: 'curl' as const, label: 'cURL' }
]
const activeLanguage = ref<ApiSnippetLanguage>('curl')

const steps = computed(() => [
  {
    id: 'key',
    done: props.hasKey,
    title: t('dashboard.onboarding.stepKeyTitle'),
    description: t('dashboard.onboarding.stepKeyDesc'),
    action: t('dashboard.onboarding.stepKeyAction'),
    to: '/keys'
  },
  {
    id: 'balance',
    done: props.hasBalance,
    title: t('dashboard.onboarding.stepBalanceTitle'),
    description: t('dashboard.onboarding.stepBalanceDesc'),
    action: t('dashboard.onboarding.stepBalanceAction'),
    to: '/purchase'
  },
  {
    id: 'request',
    done: props.hasRequest,
    title: t('dashboard.onboarding.stepRequestTitle'),
    description: t('dashboard.onboarding.stepRequestDesc'),
    action: t('dashboard.onboarding.stepRequestAction'),
    to: '/docs'
  }
])

const doneCount = computed(() => steps.value.filter((step) => step.done).length)
const progressPercent = computed(() => Math.round((doneCount.value / steps.value.length) * 100))

/** Public gateway base URL: the api_base_url setting, else the app base (origin + sub-path). */
const gatewayBase = computed(() => getPublicApiBaseUrl(appStore.cachedPublicSettings?.api_base_url))

/** OpenAI-compatible base — the protocol the cURL example targets. */
const openAIBase = computed(() => `${gatewayBase.value}/v1`)

/**
 * The example is generated by the shared snippet builder, so this block and the
 * "API key created" dialog always show the same code.
 */
const snippet = computed(() =>
  buildApiSnippet({
    protocol: 'openai',
    language: activeLanguage.value,
    baseURL: openAIBase.value,
    apiKey: insertedKey.value || KEY_PLACEHOLDER
  })
)

/** Only keys with a stored encrypted copy can be revealed. */
const revealableKeyId = computed(() => (props.firstKey?.has_full_key ? props.firstKey.id : null))

/**
 * Swap the placeholder for the account's own key. Deliberately an explicit
 * action: the dashboard must never print a secret on its own.
 */
async function insertMyKey() {
  const id = revealableKeyId.value
  if (!id || revealing.value) return
  revealing.value = true
  try {
    insertedKey.value = await keysAPI.reveal(id)
  } catch (error) {
    console.error('Failed to reveal API key:', error)
  } finally {
    revealing.value = false
  }
}

async function copy(value: string, kind: string) {
  if (!value) return
  const ok = await copyToClipboard(value)
  if (ok !== false) {
    copied.value = kind
    setTimeout(() => (copied.value = ''), 1500)
  }
}
</script>
