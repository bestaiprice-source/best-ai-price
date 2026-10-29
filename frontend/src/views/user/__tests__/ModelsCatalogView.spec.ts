import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

const { tCalls } = vi.hoisted(() => ({ tCalls: [] as Array<[string, unknown]> }))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: unknown) => {
      tCalls.push([key, params])
      return key
    }
  })
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
      available: true,
      availability_source: 'models',
      availability_verified: true,
      availability_checked_at: '2026-03-04T05:06:07Z'
    },
    {
      // Priced, but no upstream model list confirms it: the dot must NOT claim
      // it is usable.
      canonical_model_id: 'openai/gpt-5.2',
      model_id: 'gpt-5.2',
      input_price_per_1m: 0.077,
      output_price_per_1m: 0.231,
      available: true,
      availability_source: 'vendor',
      availability_verified: false
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

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() })
}))

// Model health is fetched by its own module-level composable; keep this spec
// offline and on the catalog's binary availability dot.
vi.mock('@/api/modelHealth', () => ({
  getModelHealth: vi.fn().mockResolvedValue({ items: [], window_days: 7 })
}))

vi.mock('@/components/icons/Icon.vue', () => ({
  default: { name: 'Icon', props: ['name'], template: '<i />' }
}))

import ModelsCatalogView from '../ModelsCatalogView.vue'

const mountView = async () => {
  const wrapper = mount(ModelsCatalogView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        // The cards are router-links; render them as anchors carrying the target.
        RouterLink: { props: ['to'], template: '<a :data-to="to"><slot /></a>' }
      }
    }
  })
  // Let the mocked catalog fetch resolve.
  await new Promise((resolve) => setTimeout(resolve, 0))
  await wrapper.vm.$nextTick()
  return wrapper
}

const cardsOf = (wrapper: any) => wrapper.findAll('.model-card')

describe('ModelsCatalogView', () => {
  it('renders one card per model with its retail price', async () => {
    const wrapper = await mountView()
    const cards = cardsOf(wrapper)

    expect(cards.length).toBe(3)
    // A model without any published price is hidden entirely.
    expect(wrapper.text()).not.toContain('deepseek-v9-unpriced')
    expect(wrapper.text()).toContain('claude-sonnet-5')
    expect(wrapper.text()).toContain('$0.1941/1M')
    // A flat-rate media model shows its per-unit price instead of token prices.
    expect(wrapper.text()).toContain('$0.0312')
    expect(wrapper.text()).toContain('request')
  })

  it('makes every card a link to the API keys page', async () => {
    const wrapper = await mountView()

    const targets = cardsOf(wrapper).map((card: any) => card.attributes('data-to'))
    expect(targets).toEqual(['/keys', '/keys', '/keys'])
    expect(cardsOf(wrapper)[0].text()).toContain('modelsCatalog.useKey')
  })

  it('groups every model under its provider in the names view', async () => {
    const wrapper = await mountView()
    const namesButton = wrapper.findAll('button').find((b) => b.text() === 'modelsCatalog.viewNames')!
    await namesButton.trigger('click')

    expect(wrapper.text()).toContain('Claude')
    expect(wrapper.text()).toContain('GPT')
    expect(wrapper.text()).toContain('Gemini')
    expect(wrapper.findAll('h3').length).toBe(3)
  })

  it('explains the discount badge on hover, with the percentage', async () => {
    tCalls.length = 0
    const wrapper = await mountView()

    const badge = wrapper.findAll('span').find((s: any) => s.text() === '-90%')
    expect(badge).toBeTruthy()
    expect(badge!.attributes('title')).toBe('modelsCatalog.discountTitle')
    // The percentage is interpolated, so the tooltip reads "−90% from the official price".
    expect(tCalls).toContainEqual(['modelsCatalog.discountTitle', { percent: 90 }])
  })

  it('promises usability only when an upstream model list confirms the model', async () => {
    const wrapper = await mountView()

    const dotFor = (modelId: string) => {
      const card = cardsOf(wrapper).find((c: any) => c.text().includes(modelId))!
      return card.find('span.inline-block.h-2.w-2')
    }

    // Verified by a live upstream list: green, and the tooltip says when the
    // list was checked.
    const verified = dotFor('claude-sonnet-5')
    expect(verified.classes()).toContain('bg-emerald-500')
    expect(verified.attributes('title')).toContain('modelsCatalog.availabilityAvailableTitle')
    expect(verified.attributes('title')).toContain('modelsCatalog.availabilityCheckedAt')

    // Priced but unconfirmed: amber, never green.
    const unverified = dotFor('gpt-5.2')
    expect(unverified.classes()).toContain('bg-amber-400')
    expect(unverified.attributes('title')).toBe('modelsCatalog.availabilityUnverifiedTitle')

    // Not available at all: grey.
    const unavailable = dotFor('gemini-2.5-flash-image')
    expect(unavailable.classes()).toContain('bg-gray-300')
    expect(unavailable.attributes('title')).toBe('modelsCatalog.availabilityUnavailableTitle')
  })

  it('documents the dot colours in a legend', async () => {
    const wrapper = await mountView()

    expect(wrapper.text()).toContain('modelsCatalog.availabilityAvailable')
    expect(wrapper.text()).toContain('modelsCatalog.availabilityUnverified')
    expect(wrapper.text()).toContain('modelsCatalog.availabilityUnavailable')
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