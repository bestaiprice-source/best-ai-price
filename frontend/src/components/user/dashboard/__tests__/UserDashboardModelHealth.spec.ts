import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => (params ? `${key}:${JSON.stringify(params)}` : key) })
}))

vi.mock('@/api/modelHealth', () => ({
  getModelHealth: vi.fn()
}))

import { getModelHealth } from '@/api/modelHealth'
import UserDashboardModelHealth from '../UserDashboardModelHealth.vue'
import { resetModelHealthCache } from '@/composables/useModelHealth'
import type { ModelHealthItem } from '@/api/modelHealth'

const item = (overrides: Partial<ModelHealthItem>): ModelHealthItem => ({
  model: 'm',
  candidates: ['m'],
  status: 'operational',
  availability_pct: 99.44,
  avg_latency_ms: 800,
  provider: 'anthropic',
  monitor_id: 1,
  monitor_name: 'OpenLux',
  maintenance_until: null,
  maintenance_note: '',
  ...overrides
})

const mockedGet = vi.mocked(getModelHealth)

const mountBlock = () =>
  mount(UserDashboardModelHealth, {
    global: {
      stubs: { RouterLink: { props: ['to'], template: '<a :data-to="to"><slot /></a>' } }
    }
  })

describe('UserDashboardModelHealth', () => {
  beforeEach(() => {
    resetModelHealthCache()
    mockedGet.mockReset()
  })

  it('renders nothing when no monitor reports health', async () => {
    mockedGet.mockResolvedValue({ window_days: 7, items: [] })
    const wrapper = mountBlock()
    await flushPromises()
    expect(wrapper.find('.bap-card').exists()).toBe(false)
  })

  it('lists models worst uptime first and shows the probe window', async () => {
    mockedGet.mockResolvedValue({
      window_days: 7,
      items: [
        item({ model: 'healthy-model', availability_pct: 99.98, candidates: ['healthy-model'] }),
        item({ model: 'flaky-model', availability_pct: 82.5, candidates: ['flaky-model'] })
      ]
    })
    const wrapper = mountBlock()
    await flushPromises()

    const rows = wrapper.findAll('li').map((row) => row.text())
    expect(rows).toHaveLength(2)
    expect(rows[0]).toContain('flaky-model')
    expect(rows[0]).toContain('82.50%')
    expect(rows[1]).toContain('healthy-model')
    expect(wrapper.text()).toContain('"window":7')
  })

  it('keeps models without uptime (maintenance) at the bottom', async () => {
    mockedGet.mockResolvedValue({
      window_days: 7,
      items: [
        item({ model: 'maintained', status: 'maintenance', availability_pct: null, candidates: ['maintained'] }),
        item({ model: 'serving', status: 'operational', availability_pct: 99, candidates: ['serving'] })
      ]
    })
    const wrapper = mountBlock()
    await flushPromises()

    const rows = wrapper.findAll('li').map((row) => row.text())
    expect(rows[0]).toContain('serving')
    expect(rows[1]).toContain('maintained')
    // A maintenance window must not render a misleading 0.00%.
    expect(rows[1]).not.toContain('%')
  })

  it('caps the list and points at the full screen', async () => {
    mockedGet.mockResolvedValue({
      window_days: 7,
      items: Array.from({ length: 9 }, (_, i) =>
        item({ model: `model-${i}`, availability_pct: 90 + i, candidates: [`model-${i}`] })
      )
    })
    const wrapper = mountBlock()
    await flushPromises()

    expect(wrapper.findAll('li')).toHaveLength(6)
    expect(wrapper.text()).toContain('"count":3')
    expect(wrapper.find('[data-to="/monitor"]').exists()).toBe(true)
  })
})
