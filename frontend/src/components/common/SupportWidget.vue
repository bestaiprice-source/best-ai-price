<template>
  <div class="support-widget">
    <!-- Floating trigger -->
    <button
      type="button"
      class="fixed bottom-5 right-5 z-40 inline-flex h-12 w-12 items-center justify-center rounded-full bg-primary-600 text-white shadow-lg transition-transform hover:scale-105"
      :title="t('support.consultantTitle')"
      @click="open = !open"
    >
      <Icon :name="open ? 'x' : 'chat'" size="md" />
    </button>

    <!-- Panel -->
    <div
      v-if="open"
      class="fixed bottom-20 right-5 z-40 w-[380px] max-w-[calc(100vw-2.5rem)] overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-800"
    >
      <div class="flex items-center justify-between border-b border-gray-100 px-4 py-3 dark:border-dark-700">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('support.consultantTitle') }}</h3>
        <button class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" @click="open = false">
          <Icon name="x" size="sm" />
        </button>
      </div>

      <div class="max-h-[65vh] overflow-y-auto p-4">
        <p class="text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('support.consultantIntro') }}</p>

        <!-- Message -->
        <textarea
          v-model="message"
          rows="4"
          class="input mt-3 w-full resize-none text-sm"
          :placeholder="t('support.messagePlaceholder')"
        ></textarea>

        <!-- Optional request reference (attached to the message automatically) -->
        <div class="mt-3 rounded-xl border border-gray-100 p-3 dark:border-dark-700">
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
              :placeholder="t('support.modelPlaceholder')"
            />
          </div>
          <p class="mt-2 text-[11px] text-gray-400">{{ t('support.noSecrets') }}</p>
        </div>

        <!-- Actions -->
        <button
          class="mt-3 w-full rounded-lg bg-primary-600 px-3 py-2.5 text-sm font-medium text-white transition-colors hover:bg-primary-500 disabled:cursor-not-allowed disabled:opacity-50"
          :disabled="!message.trim()"
          @click="sendToConsultant"
        >
          {{ t('support.sendToConsultant') }}
        </button>
        <button
          class="mt-2 w-full rounded-lg border border-gray-200 px-3 py-2 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
          @click="copyContext"
        >
          {{ copied ? t('support.copied') : t('support.copyContext') }}
        </button>

        <p v-if="sentNote" class="mt-2 text-xs text-emerald-600 dark:text-emerald-400">{{ sentNote }}</p>
        <p v-if="!contactRaw" class="mt-2 text-xs text-amber-600 dark:text-amber-400">{{ t('support.notConfigured') }}</p>
        <p v-else class="mt-2 text-[11px] text-gray-400">{{ contactHint }}</p>
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
const message = ref('')
const requestId = ref('')
const modelName = ref('')
const copied = ref(false)
const sentNote = ref('')

const contactRaw = computed(() => String(appStore.cachedPublicSettings?.contact_info || appStore.contactInfo || '').trim())
const contactHref = computed(() => {
  const raw = contactRaw.value
  if (!raw) return ''
  if (/^https?:\/\//i.test(raw)) return raw
  if (raw.includes('@') && !raw.includes(' ')) return `mailto:${raw}`
  return ''
})
const contactHint = computed(() => t('support.contactHint'))

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

/** The message the consultant receives: the user's own words + the safe context. */
function buildMessage(): string {
  return [message.value.trim(), '', '---', contextText.value].join('\n')
}

async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch (e) {
    console.warn('Copy failed:', e)
    return false
  }
}

async function copyContext() {
  if (await copyText(buildMessage())) {
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  }
}

/**
 * Hand the message to the human consultant. Email contacts get a prefilled mail
 * (the context rides along automatically); chat/URL contacts get the message
 * copied to the clipboard and the chat opened, since direct-chat links cannot
 * carry a prefilled body.
 */
async function sendToConsultant() {
  sentNote.value = ''
  const text = buildMessage()
  const href = contactHref.value
  if (!contactRaw.value) {
    return
  }
  if (href.startsWith('mailto:')) {
    const subject = encodeURIComponent(t('support.mailSubject'))
    window.location.href = `${href}?subject=${subject}&body=${encodeURIComponent(text)}`
    sentNote.value = t('support.mailOpened')
    return
  }
  if (href) {
    await copyText(text)
    window.open(href, '_blank', 'noopener,noreferrer')
    sentNote.value = t('support.messageCopied')
    return
  }
  // Bare handle (e.g. a Telegram username): copy the message and show the handle.
  await copyText(text)
  sentNote.value = t('support.messageCopiedContact', { contact: contactRaw.value })
}
</script>