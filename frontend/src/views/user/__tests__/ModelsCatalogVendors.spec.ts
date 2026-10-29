import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

// The view pulls in AppLayout, which reaches for the app-wide i18n singleton.
vi.mock('@/i18n', () => ({
  i18n: { global: { t: (key: string) => key, locale: { value: 'en' } } }
}))

// Fixture for the vendor grouping: known prefixes (whose raw key used to be
// printed with CSS `capitalize` -> "Openai", "Zhipu", …) plus several models the
// gateway canonicalizes under the generic `other/` prefix, which must NOT all
// collapse into one "Other" chip.
vi.mock('@/api/publicModels', () => ({
  getPublicModels: vi.fn().mockResolvedValue([
    { canonical_model_id: 'openai/gpt-5.2', input_price_per_1m: 1, output_price_per_1m: 2 },
    { canonical_model_id: 'zhipu/glm-4.6', input_price_per_1m: 1, output_price_per_1m: 2 },
    { canonical_model_id: 'moonshot/kimi-latest', input_price_per_1m: 1, output_price_per_1m: 2 },
    { canonical_model_id: 'xai/grok-4', input_price_per_1m: 1, output_price_per_1m: 2 },
    { canonical_model_id: 'baai/bge-m3', input_price_per_1m: 1, output_price_per_1m: 2 },
    // Recovered from `other/` by slug.
    { canonical_model_id: 'other/doubao-seedream-5-0-260128', native_price_usd: 0.018, native_price_unit: 'request' },
    { canonical_model_id: 'other/mj_imagine', native_price_usd: 0.048, native_price_unit: 'request' },
    { canonical_model_id: 'other/speech-02-hd', native_price_usd: 0.0012, native_price_unit: 'request' },
    { canonical_model_id: 'other/veo_3_1', native_price_usd: 0.12, native_price_unit: 'request' },
    // Provider model names carrying no vendor prefix: TypeSafe AI's "jev" and
    // Tencent's Hunyuan "hy4" (verified against the provider's own metadata).
    { canonical_model_id: 'other/jev-1.14.0', native_price_usd: 0.0021, native_price_unit: 'request' },
    { canonical_model_id: 'other/hy4-preview', native_price_usd: 0.0021, native_price_unit: 'request' }
  ])
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() })
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
        RouterLink: { props: ['to'], template: '<a :data-to="to"><slot /></a>' }
      }
    }
  })
  await new Promise((resolve) => setTimeout(resolve, 0))
  await wrapper.vm.$nextTick()
  return wrapper
}

/** The provider chips — the leading "all providers" chip has no "(N)" suffix. */
const chips = (wrapper: any) =>
  wrapper
    .findAll('button')
    .map((b: any) => b.text())
    .filter((text: string) => / \(\d+\)$/.test(text))

describe('ModelsCatalogView vendor labels', () => {
  it('shows real brand names, never the raw capitalized prefix', async () => {
    const wrapper = await mountView()
    const texts = chips(wrapper)

    // Proper brand casing + the product name users recognise.
    expect(texts).toContain('OpenAI (1)')
    expect(texts).toContain('GLM (1)')
    expect(texts).toContain('KIMI (1)')
    expect(texts).toContain('xAI (1)')
    expect(texts).toContain('BAAI (1)')
    // Never the naive CSS-capitalize output.
    expect(wrapper.text()).not.toContain('Openai')
    expect(wrapper.text()).not.toContain('Zhipu')
    expect(wrapper.text()).not.toContain('Moonshot')
  })

  it('splits the generic "other/" bucket into the real vendors', async () => {
    const wrapper = await mountView()
    const texts = chips(wrapper)

    expect(texts).toContain('ByteDance (1)')
    expect(texts).toContain('Midjourney (1)')
    // speech-* and aigc-video-hailuo are MiniMax; veo_* is Google.
    expect(texts).toContain('MiniMax (1)')
    expect(texts).toContain('Google (1)')
    // No leftover catch-all bucket.
    expect(texts.some((t: string) => t.startsWith('Other '))).toBe(false)
  })

  it('names the vendors the provider leaves without a prefix', async () => {
    const wrapper = await mountView()
    const texts = chips(wrapper)

    // `jev-*` is TypeSafe AI's System One; `hy4-preview` is Tencent's Hunyuan Hy4.
    expect(texts).toContain('TypeSafe (1)')
    expect(texts).toContain('Tencent (1)')
    // The raw slug token must never be used as a label.
    expect(wrapper.text()).not.toContain('Jev')
    expect(wrapper.text()).not.toContain('Hy4')
  })

  it('orders the pinned vendors first, then the rest by count then name', async () => {
    const wrapper = await mountView()

    expect(chips(wrapper)).toEqual([
      'OpenAI (1)',
      'Google (1)',
      'BAAI (1)',
      'ByteDance (1)',
      'Midjourney (1)',
      'MiniMax (1)',
      'KIMI (1)',
      'Tencent (1)',
      'TypeSafe (1)',
      'xAI (1)',
      'GLM (1)'
    ])
  })

  it('uses the same labels for the All-models group headers', async () => {
    const wrapper = await mountView()
    const namesButton = wrapper.findAll('button').find((b) => b.text() === 'modelsCatalog.viewNames')!
    await namesButton.trigger('click')

    const headers = wrapper.findAll('h3').map((h: any) => h.text())
    expect(headers).toContain('GLM')
    expect(headers).toContain('KIMI')
    expect(headers).toContain('ByteDance')
    expect(headers).toContain('Midjourney')
  })
})
