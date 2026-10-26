import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import SupportWidget from '@/components/common/SupportWidget.vue'
import { openSupportWidget, useSupportWidget } from '@/composables/useSupportWidget'
import { sendSupportMessage } from '@/api/support'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ fullPath: '/purchase' }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ cachedPublicSettings: { contact_info: '' }, contactInfo: '' }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isAuthenticated: true, user: { id: 7 } }),
}))

vi.mock('@/api/support', () => ({
  getSupportThread: vi.fn().mockResolvedValue({ items: [], unread: 0 }),
  markSupportRead: vi.fn().mockResolvedValue(undefined),
  sendSupportMessage: vi.fn().mockResolvedValue({ delivered: true }),
}))

const { open, prefill, topic, amount } = useSupportWidget()

describe('SupportWidget', () => {
  beforeEach(() => {
    open.value = false
    prefill.value = ''
    topic.value = ''
    amount.value = ''
    vi.mocked(sendSupportMessage).mockClear()
  })

  it('stays closed until something asks for it, then opens pre-filled', async () => {
    const wrapper = mount(SupportWidget)
    expect(wrapper.find('textarea').exists()).toBe(false)

    openSupportWidget('Hello! Please issue a manual invoice for a 100.00 top-up.')
    await nextTick()

    const textarea = wrapper.find('textarea')
    expect(textarea.exists()).toBe(true)
    expect((textarea.element as HTMLTextAreaElement).value).toBe(
      'Hello! Please issue a manual invoice for a 100.00 top-up.',
    )
    // The pre-fill is consumed, so reopening the widget later does not re-fill.
    expect(prefill.value).toBe('')

    wrapper.unmount()
  })

  it('opening with no message keeps whatever the user had typed', async () => {
    const wrapper = mount(SupportWidget)
    openSupportWidget('first draft')
    await nextTick()
    open.value = false
    await nextTick()

    openSupportWidget()
    await nextTick()

    expect((wrapper.find('textarea').element as HTMLTextAreaElement).value).toBe('first draft')
    wrapper.unmount()
  })

  it('sends an invoice request with its topic and amount, then clears them', async () => {
    const wrapper = mount(SupportWidget)
    openSupportWidget('invoice please', { topic: 'manual_invoice', amount: '250.00' })
    await nextTick()

    const sendBtn = wrapper.findAll('button').find(b => b.text() === 'support.sendToConsultant')
    expect(sendBtn).toBeTruthy()
    await sendBtn!.trigger('click')
    await flushPromises()

    expect(vi.mocked(sendSupportMessage)).toHaveBeenCalledWith(
      expect.objectContaining({
        message: 'invoice please',
        topic: 'manual_invoice',
        amount: '250.00',
        page: '/purchase',
      }),
    )
    // The tag must not leak onto the next, unrelated message.
    expect(topic.value).toBe('')
    expect(amount.value).toBe('')
    wrapper.unmount()
  })
})
