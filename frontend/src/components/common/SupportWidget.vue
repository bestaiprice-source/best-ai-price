<template>
  <div class="support-widget">
    <!-- Floating trigger -->
    <button
      type="button"
      class="fixed bottom-5 right-5 z-40 inline-flex h-12 w-12 items-center justify-center rounded-full bg-primary-600 text-white shadow-lg transition-transform hover:scale-105"
      :title="t('support.consultantTitle')"
      @click="toggleOpen"
    >
      <Icon :name="open ? 'x' : 'chat'" size="md" />
      <span
        v-if="unread > 0 && !open"
        class="absolute -right-1 -top-1 flex h-5 min-w-[1.25rem] items-center justify-center rounded-full bg-red-500 px-1 text-[11px] font-semibold text-white"
        :title="t('support.newReply')"
      >{{ unread }}</span>
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

        <!-- Conversation: the user's messages and the consultant's Telegram replies -->
        <div v-if="thread.length" ref="threadEl" class="mt-3 max-h-56 space-y-2 overflow-y-auto rounded-xl bg-gray-50 p-2 dark:bg-dark-900">
          <div
            v-for="m in thread"
            :key="m.id"
            class="flex flex-col"
            :class="m.direction === 'user' ? 'items-end' : 'items-start'"
          >
            <span class="mb-0.5 text-[10px] uppercase tracking-wide text-gray-400">
              {{ m.direction === 'user' ? t('support.youLabel') : t('support.consultantLabel') }}
            </span>
            <div
              class="max-w-[85%] whitespace-pre-wrap break-words rounded-xl px-3 py-1.5 text-xs"
              :class="m.direction === 'user'
                ? 'bg-primary-600 text-white'
                : 'bg-white text-gray-800 shadow-sm dark:bg-dark-700 dark:text-gray-100'"
            >{{ m.body }}</div>
          </div>
        </div>

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
          :disabled="!message.trim() || sending"
          @click="sendToConsultant"
        >
          {{ sending ? t('support.sending') : t('support.sendToConsultant') }}
        </button>
        <button
          class="mt-2 w-full rounded-lg border border-gray-200 px-3 py-2 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
          @click="copyContext"
        >
          {{ copied ? t('support.copied') : t('support.copyContext') }}
        </button>

        <p v-if="sentNote" class="mt-2 text-xs text-emerald-600 dark:text-emerald-400">{{ sentNote }}</p>
        <p v-if="errorNote" class="mt-2 text-xs text-red-600 dark:text-red-400">{{ errorNote }}</p>
        <p v-if="!contactRaw && !chatEnabled" class="mt-2 text-xs text-amber-600 dark:text-amber-400">{{ t('support.notConfigured') }}</p>
        <p v-else-if="contactRaw" class="mt-2 text-[11px] text-gray-400">{{ contactHint }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useSupportWidget } from '@/composables/useSupportWidget'
import {
  getSupportThread,
  markSupportRead,
  sendSupportMessage,
  type SupportMessage,
} from '@/api/support'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()

const { open, prefill, topic, amount } = useSupportWidget()
const message = ref('')
const requestId = ref('')
const modelName = ref('')
const copied = ref(false)
const sentNote = ref('')
const errorNote = ref('')
const sending = ref(false)
const thread = ref<SupportMessage[]>([])
const unread = ref(0)
const threadEl = ref<HTMLElement | null>(null)

// Signed-in users talk to the consultant through the in-app chat (forwarded to
// the admin Telegram bot); guests keep the mail / copy fallback below.
const chatEnabled = computed(() => authStore.isAuthenticated)

const POLL_OPEN_MS = 10_000
const POLL_CLOSED_MS = 60_000
let pollTimer: ReturnType<typeof setTimeout> | null = null

async function refreshThread() {
  if (!chatEnabled.value) return
  try {
    const res = await getSupportThread()
    thread.value = res.items
    unread.value = res.unread
    if (open.value && res.unread > 0) {
      await markSupportRead().catch(() => undefined)
      unread.value = 0
    }
    await nextTick()
    if (threadEl.value) threadEl.value.scrollTop = threadEl.value.scrollHeight
  } catch (e) {
    console.warn('Support thread refresh failed:', e)
  }
}

function schedulePoll() {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = setTimeout(async () => {
    if (document.visibilityState === 'visible') await refreshThread()
    schedulePoll()
  }, open.value ? POLL_OPEN_MS : POLL_CLOSED_MS)
}

function toggleOpen() {
  open.value = !open.value
}

watch(open, (isOpen) => {
  if (isOpen) void refreshThread()
  schedulePoll()
})
// Another screen asked us to talk to the consultant (e.g. "write to us" on the
// top-up page): the widget opens itself and the message box is pre-filled.
watch(prefill, (text) => {
  if (!text) return
  message.value = text
  prefill.value = ''
})
watch(chatEnabled, (enabled) => {
  if (enabled) void refreshThread()
  else {
    thread.value = []
    unread.value = 0
  }
})
onMounted(() => {
  void refreshThread()
  schedulePoll()
})
onBeforeUnmount(() => {
  if (pollTimer) clearTimeout(pollTimer)
})

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
  errorNote.value = ''
  // Built before the input is cleared so the fallback flow can still use it.
  const text = buildMessage()

  if (chatEnabled.value) {
    sending.value = true
    try {
      const res = await sendSupportMessage({
        message: message.value.trim(),
        request_id: requestId.value.trim(),
        model: modelName.value.trim(),
        page: route.fullPath,
        // Topic/amount only shape the admin's Telegram copy; they are cleared
        // below so a later, unrelated message is not tagged with them.
        topic: topic.value,
        amount: amount.value,
      })
      message.value = ''
      topic.value = ''
      amount.value = ''
      await refreshThread()
      if (res.delivered) {
        sentNote.value = t('support.sentToTelegram')
        return
      }
      // Stored but Telegram is unreachable: fall through to the mail/copy flow.
      errorNote.value = t('support.deliveryFallback')
    } catch (e: unknown) {
      const status = (e as { status?: number; response?: { status?: number } })?.status
        ?? (e as { response?: { status?: number } })?.response?.status
      errorNote.value = status === 429 ? t('support.rateLimited') : t('support.sendFailed')
      return
    } finally {
      sending.value = false
    }
  }

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