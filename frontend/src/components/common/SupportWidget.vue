<template>
  <div class="support-widget">
    <!-- Floating trigger -->
    <button
      type="button"
      class="fixed bottom-5 right-5 z-40 inline-flex h-12 w-12 items-center justify-center rounded-full bg-primary-600 text-white shadow-lg transition-transform hover:scale-105"
      :title="t('support.title')"
      @click="open = !open"
    >
      <Icon :name="open ? 'x' : 'chat'" size="md" />
    </button>

    <!-- Panel -->
    <div
      v-if="open"
      class="fixed bottom-20 right-5 z-40 w-[360px] max-w-[calc(100vw-2.5rem)] overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-800"
    >
      <div class="flex items-center justify-between border-b border-gray-100 px-4 py-3 dark:border-dark-700">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('support.title') }}</h3>
        <button class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" @click="open = false">
          <Icon name="x" size="sm" />
        </button>
      </div>

      <div class="max-h-[60vh] overflow-y-auto p-4">
        <!-- FAQ -->
        <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('support.faq') }}</p>
        <div class="mt-2 divide-y divide-gray-100 dark:divide-dark-700">
          <div v-for="(faq, i) in faqs" :key="i" class="py-2">
            <button class="flex w-full items-center justify-between text-left text-sm font-medium text-gray-800 dark:text-gray-200" @click="toggleFaq(i)">
              <span>{{ faq.q }}</span>
              <span class="text-gray-400">{{ expanded === i ? '−' : '+' }}</span>
            </button>
            <p v-if="expanded === i" class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ faq.a }}</p>
          </div>
        </div>

        <!-- Contact -->
        <div v-if="contactHref" class="mt-4 rounded-xl bg-gray-50 p-3 dark:bg-dark-900">
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('support.contactHint') }}</p>
          <a :href="contactHref" target="_blank" rel="noopener noreferrer" class="mt-1 inline-block text-sm font-medium text-primary-600 hover:underline">
            {{ contactText }}
          </a>
        </div>

        <!-- Dispute context -->
        <div class="mt-4 rounded-xl border border-gray-100 p-3 dark:border-dark-700">
          <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('support.dispute') }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('support.disputeHint') }}</p>
          <div class="mt-2 grid grid-cols-2 gap-2">
            <input
              v-model="requestId"
              type="text"
              class="input w-full text-xs"
              :placeholder="t('support.requestIdPlaceholder')"
            />
            <input
              v-model="modelName"
              type="text"
              class="input w-full text-xs"
              placeholder="Model (optional, e.g. gpt-4o)"
            />
          </div>
          <pre class="mt-2 overflow-x-auto rounded-lg bg-gray-900 p-3 text-[11px] leading-relaxed text-gray-100"><code>{{ contextText }}</code></pre>
          <button
            class="mt-2 w-full rounded-lg bg-primary-600 px-3 py-2 text-xs font-medium text-white transition-colors hover:bg-primary-500"
            @click="copyContext"
          >
            {{ copied ? t('support.copied') : t('support.copyContext') }}
          </button>
          <p class="mt-1 text-[11px] text-gray-400">{{ t('support.noSecrets') }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()

const open = ref(false)
const expanded = ref<number | null>(0)
const requestId = ref('')
const modelName = ref('')
const copied = ref(false)

function toggleFaq(i: number) {
  expanded.value = expanded.value === i ? null : i
}

const faqs = computed(() => [
  { q: t('support.faq_topup_q'), a: t('support.faq_topup_a') },
  { q: t('support.faq_key_q'), a: t('support.faq_key_a') },
  { q: t('support.faq_models_q'), a: t('support.faq_models_a') },
  { q: t('support.faq_errors_q'), a: t('support.faq_errors_a') },
  { q: t('support.faq_charges_q'), a: t('support.faq_charges_a') },
  { q: t('support.faq_failover_q'), a: t('support.faq_failover_a') },
  { q: t('support.faq_limits_q'), a: t('support.faq_limits_a') },
])

const contactRaw = computed(() => String(appStore.cachedPublicSettings?.contact_info || appStore.contactInfo || '').trim())
const contactHref = computed(() => {
  const raw = contactRaw.value
  if (!raw) return ''
  if (/^https?:\/\//i.test(raw)) return raw
  if (raw.includes('@') && !raw.includes(' ')) return `mailto:${raw}`
  return ''
})
const contactText = computed(() => contactRaw.value)

// Support context: only non-sensitive identifiers are ever included (§15) —
// user id, request id, model, timestamp, page. Never keys or credentials.
const contextText = computed(() => {
  const user = authStore.user
  const lines = [
    `user_id: ${user?.id ?? 'guest'}`,
    `request_id: ${requestId.value.trim() || 'n/a'}`,
    `model: ${modelName.value.trim() || 'n/a'}`,
    `page: ${route.fullPath}`,
    `timestamp: ${new Date().toISOString()}`,
  ]
  return lines.join('\n')
})

async function copyContext() {
  try {
    await navigator.clipboard.writeText(contextText.value)
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  } catch (e) {
    console.warn('Copy failed:', e)
  }
}
</script>
