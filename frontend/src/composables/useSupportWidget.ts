import { ref } from 'vue'

/**
 * Shared state of the floating consultant widget (`components/common/SupportWidget.vue`).
 *
 * The widget is mounted once in the app layout, so a screen that needs to hand the
 * user over to a human (e.g. the top-up page asking for a manual invoice) cannot
 * open it through props or events — it opens it through this module-level state.
 */
const open = ref(false)
const prefill = ref('')
const topic = ref('')
const amount = ref('')

export interface OpenSupportWidgetOptions {
  /** Backend-known topic selecting the Telegram headline (e.g. `manual_invoice`). */
  topic?: string
  /** Amount shown explicitly in the Telegram alert. */
  amount?: string
}

export function useSupportWidget() {
  return { open, prefill, topic, amount }
}

/**
 * Open the consultant widget, optionally pre-filling the message box and tagging
 * the request. An empty message leaves whatever the user had already typed
 * untouched (the tag is still applied to the next message).
 */
export function openSupportWidget(message = '', options: OpenSupportWidgetOptions = {}): void {
  if (message) prefill.value = message
  if (options.topic !== undefined) topic.value = options.topic
  if (options.amount !== undefined) amount.value = options.amount
  open.value = true
}
