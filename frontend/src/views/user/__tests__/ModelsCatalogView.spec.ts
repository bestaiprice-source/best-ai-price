import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

// The view pulls in AppLayout, which reaches for the app-wide i18n singleton.
vi.mock('@/i18n', () => ({
  i18n: { global: { t: (key: string) => key, locale: { value: 'en' } } }
}))

// Mock factories are hoisted, so the fixture must live inside the factory.
vi.mock('@/api/publicModels', () => ({
  getPublicModels: vi.fn().mockResolvedValue([
    {
      canonical_model_id: 'anthropic/claude-sonnet-5',
      model_id: 'claude-sonnet-5',
      input_price_per_1m: 0.1941,
      output_price_per_1m: 0.9706,
      discount_percent: 90,
      available: true
    },
    {
      canonical_model_id: 'openai/gpt-5.2',
      model_id: 'gpt-5.2',
      input_price_per_1m: 0.077,
      output_price_per_1m: 0.231,
      available: true
    },
    {
      canonical_model_id: 'google/gemini-2.5-flash-image',
      model_id: 'gemini-2.5-flash-image',
      native_price_usd: 0.0312,
      native_price_unit: 'request',
      available: false
    },
    // No published price at all: must never be listed.
    {
      canonical_model_id: 'deepseek/deepseek-v9-unpriced',
      model_id: 'deepseek-v9-unpriced',
      available: true
    }
  ])
}))

vi.mock('@/components/icons/Icon.vue', () => ({
  default: { name: 'Icon', props: ['name'], template: '<i />' }
}))

import ModelsCatalogView from '../ModelsCatalogView.vue'

const mountView = async () => {
  const wrapper = mount(ModelsCatalogView, {
    global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } }
  })
  // Let the mocked catalog fetch resolve.
  await new Promise((resolve) => setTimeout(resolve, 0))
  await wrapper.vm.$nextTick()
  return wrapper
}

const cardsOf = (wrapper: any) => wrapper.findAll('.grid > div')

describe('ModelsCatalogView', () => {
  it('renders one card per model with its retail price and discount badge', async () => {
    const wrapper = await mountView()
    const cards = cardsOf(wrapper)

    expect(cards.length).toBe(3)
    // A model without any published price is hidden entirely.
    expect(wrapper.text()).not.toContain('deepseek-v9-unpriced')
    expect(wrapper.text()).toContain('claude-sonnet-5')
    expect(wrapper.text()).toContain('$0.1941/1M')
    expect(wrapper.text()).toContain('-90%')
    // A flat-rate media model shows its per-unit price instead of token prices.
    expect(wrapper.text()).toContain('$0.0312')
    expect(wrapper.text()).toContain('request')
  })

  it('groups every model under its provider in the names view', async () => {
    const wrapper = await mountView()
    const namesButton = wrapper.findAll('button').find((b) => b.text() === 'modelsCatalog.viewNames')!
    await namesButton.trigger('click')

    expect(wrapper.text()).toContain('anthropic')
    expect(wrapper.text()).toContain('openai')
    expect(wrapper.text()).toContain('google')
    expect(wrapper.findAll('h3').length).toBe(3)
  })

  it('filters by type and by free-text search', async () => {
    const wrapper = await mountView()

    const imageButton = wrapper.findAll('button').find((b) => b.text() === 'modelsCatalog.typeImage')!
    await imageButton.trigger('click')
    expect(cardsOf(wrapper).length).toBe(1)
    expect(wrapper.text()).toContain('gemini-2.5-flash-image')

    // Back to all types, then search for a specific model.
    await wrapper.findAll('button').find((b) => b.text() === 'modelsCatalog.typeAll')!.trigger('click')
    await wrapper.find('input[type="text"]').setValue('gpt')
    expect(cardsOf(wrapper).length).toBe(1)
    expect(wrapper.text()).toContain('gpt-5.2')
  })
})