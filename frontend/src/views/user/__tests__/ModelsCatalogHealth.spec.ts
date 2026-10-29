import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

vi.mock('@/i18n', () => ({
  i18n: { global: { t: (key: string) => key, locale: { value: 'en' } } }
}))

vi.mock('@/api/publicModels', () => ({
  getPublicModels: vi.fn().mockResolvedValue([
    {
      canonical_model_id: 'anthropic/claude-sonnet-5',
      model_id: 'claude-sonnet-5',
      input_price_per_1m: 0.19,
      output_price_per_1m: 0.76,
      available: true
    },
    {
      // Priced with no monitor of its own: the uptime comes from real traffic.
      canonical_model_id: 'google/gemini-3.1-pro',
      model_id: 'gemini-3.1-pro',
      input_price_per_1m: 0.4,
      output_price_per_1m: 1.6,
      available: true,
      availability_verified: true
    },
    {
      // No monitor and no traffic: the card must fall back to the catalog's own
      // availability dot.
      canonical_model_id: 'openai/gpt-unmonitored',
      model_id: 'gpt-unmonitored',
      input_price_per_1m: 0.1,
      output_price_per_1m: 0.2,
      available: true,
      availability_verified: true
    }
  ])
}))

vi.mock('@/api/modelHealth', () => ({
  getModelHealth: vi.fn().mockResolvedValue({
    window_days: 7,
    items: [
      {
        // Upstream id as configured on the monitor, joined onto the catalog's
        // canonical id through the gateway-produced candidate keys.
        model: 'claude-sonnet-5-20250929',
        candidates: ['claude-sonnet-5-20250929', 'claude-sonnet-5'],
        status: 'degraded',
        availability_pct: 95.5,
        avg_latency_ms: 7200,
        provider: 'anthropic',
        monitor_id: 1,
        monitor_name: 'OpenLux Claude',
        maintenance_until: null,
        maintenance_note: '',
        source: 'probe'
      },
      {
        // Measured on the requests users actually made: no monitor covers it,
        // so the traffic row is what the card has to show.
        model: 'gemini-3.1-pro',
        candidates: ['gemini-3.1-pro', 'google/gemini-3.1-pro'],
        status: 'operational',
        availability_pct: 99.44,
        avg_latency_ms: 900,
        provider: '',
        monitor_id: 0,
        monitor_name: '',
        maintenance_until: null,
        maintenance_note: '',
        source: 'traffic',
        requests: 360
      }
    ]
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() })
}))

vi.mock('@/components/icons/Icon.vue', () => ({
  default: { name: 'Icon', props: ['name'], template: '<i />' }
}))

import ModelsCatalogView from '../ModelsCatalogView.vue'
import { resetModelHealthCache } from '@/composables/useModelHealth'

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

describe('ModelsCatalogView — model health on the cards', () => {
  beforeEach(() => {
    resetModelHealthCache()
  })

  it('shows the measured uptime on a monitored model card', async () => {
    const wrapper = await mountView()
    expect(wrapper.text()).toContain('95.50%')
    // The legend is the wording the provider consoles use.
    expect(wrapper.text()).toContain('modelHealth.status.highLatency')
  })

  it('falls back to the binary availability dot for an unmonitored model', async () => {
    const wrapper = await mountView()
    const cards = wrapper.findAll('.model-card')
    const unmonitored = cards.find((card) => card.text().includes('gpt-unmonitored'))
    expect(unmonitored).toBeDefined()
    // No uptime percentage on a card no monitor and no traffic covers.
    expect(unmonitored!.text()).not.toContain('%')
    expect(unmonitored!.find('.bg-emerald-500').exists()).toBe(true)
  })

  it('shows a traffic-measured uptime when no monitor covers the model', async () => {
    const wrapper = await mountView()
    const card = wrapper.findAll('.model-card').find((c) => c.text().includes('gemini-3.1-pro'))!

    // The provider-console number, on a model nobody configured a probe for.
    expect(card.text()).toContain('99.44%')
    const badge = card.find('span[title]')
    expect(badge.attributes('title')).toContain('modelHealth.status.normal')
    // The tooltip must not let the reader assume a continuous monitor.
    expect(badge.attributes('title')).toContain('modelHealth.sourceTraffic')
    expect(badge.attributes('title')).toContain('modelHealth.trafficSample')
  })
})
