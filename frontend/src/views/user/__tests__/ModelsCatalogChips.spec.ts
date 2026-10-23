import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

// The view pulls in AppLayout, which reaches for the app-wide i18n singleton.
vi.mock('@/i18n', () => ({
  i18n: { global: { t: (key: string) => key, locale: { value: 'en' } } }
}))

// Fixture tuned for the provider chips: openai 3 text models, google 1 text +
// 1 image model, anthropic 1 text model, and one model with no vendor prefix
// (the `other` bucket). Mock factories are hoisted, so the fixture must live
// inside the factory.
vi.mock('@/api/publicModels', () => ({
  getPublicModels: vi.fn().mockResolvedValue([
    { canonical_model_id: 'openai/gpt-5.2', input_price_per_1m: 0.1, output_price_per_1m: 0.2 },
    { canonical_model_id: 'openai/gpt-5.2-mini', input_price_per_1m: 0.1, output_price_per_1m: 0.2 },
    { canonical_model_id: 'openai/o3', input_price_per_1m: 0.1, output_price_per_1m: 0.2 },
    { canonical_model_id: 'google/gemini-2.5-pro', input_price_per_1m: 0.1, output_price_per_1m: 0.2 },
    { canonical_model_id: 'google/gemini-2.5-flash-image', native_price_usd: 0.03, native_price_unit: 'request' },
    { canonical_model_id: 'anthropic/claude-sonnet-5', input_price_per_1m: 0.1, output_price_per_1m: 0.2 },
    { model_id: 'vendorless-model', input_price_per_1m: 0.1, output_price_per_1m: 0.2 }
  ])
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
        // The cards are router-links; render them as anchors.
        RouterLink: { props: ['to'], template: '<a :data-to="to"><slot /></a>' }
      }
    }
  })
  // Let the mocked catalog fetch resolve.
  await new Promise((resolve) => setTimeout(resolve, 0))
  await wrapper.vm.$nextTick()
  return wrapper
}

const buttonByText = (wrapper: any, text: string) =>
  wrapper.findAll('button').find((b: any) => b.text() === text)!

/** The provider chips — the leading "all providers" chip has no "(N)" suffix. */
const chips = (wrapper: any) =>
  wrapper
    .findAll('button')
    .map((b: any) => b.text())
    .filter((text: string) => / \(\d+\)$/.test(text))

const cardsOf = (wrapper: any) => wrapper.findAll('.model-card')

describe('ModelsCatalogView provider chips', () => {
  it('orders them by model count, alphabetically on ties, "other" last', async () => {
    const wrapper = await mountView()

    expect(chips(wrapper)).toEqual([
      'openai (3)',
      'google (2)',
      'anthropic (1)',
      'other (1)'
    ])
  })

  it('recounts the chips for the active type filter and drops the empty ones', async () => {
    const wrapper = await mountView()

    await buttonByText(wrapper, 'modelsCatalog.typeImage').trigger('click')

    // Only google still has a model, so only its chip is offered — and the list
    // shows exactly the one model the chip promises.
    expect(chips(wrapper)).toEqual(['google (1)'])
    expect(cardsOf(wrapper).length).toBe(1)
    expect(wrapper.text()).toContain('gemini-2.5-flash-image')
  })

  it('recounts the chips for the search box', async () => {
    const wrapper = await mountView()

    await wrapper.find('input[type="text"]').setValue('gpt')

    expect(chips(wrapper)).toEqual(['openai (2)'])
    expect(cardsOf(wrapper).length).toBe(2)
  })

  it('keeps the selected provider chip visible even when the filter empties it', async () => {
    const wrapper = await mountView()

    await buttonByText(wrapper, 'openai (3)').trigger('click')
    expect(cardsOf(wrapper).length).toBe(3)

    // openai has no image model: its chip must survive so the filter can be
    // cleared again.
    await buttonByText(wrapper, 'modelsCatalog.typeImage').trigger('click')
    expect(chips(wrapper)).toEqual(['google (1)', 'openai (0)'])

    // Clearing the type filter brings the other providers back.
    await buttonByText(wrapper, 'modelsCatalog.typeAll').trigger('click')
    expect(chips(wrapper)).toEqual([
      'openai (3)',
      'google (2)',
      'anthropic (1)',
      'other (1)'
    ])
  })
})
