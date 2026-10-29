import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

vi.mock('@/components/icons/Icon.vue', () => ({
  default: { name: 'Icon', props: ['name'], template: '<i />' }
}))

import UserDashboardStats from '../UserDashboardStats.vue'
import SparkLine from '../SparkLine.vue'
import StatDelta from '../StatDelta.vue'
import type { UserDashboardStats as UserStatsType } from '@/api/usage'
import type { TrendDataPoint } from '@/types'

const stats: UserStatsType = {
  total_api_keys: 2,
  active_api_keys: 1,
  total_requests: 100,
  total_input_tokens: 1000,
  total_output_tokens: 500,
  total_cache_creation_tokens: 0,
  total_cache_read_tokens: 0,
  total_tokens: 1500,
  total_cost: 1,
  total_actual_cost: 1,
  today_requests: 30,
  today_input_tokens: 300,
  today_output_tokens: 100,
  today_cache_creation_tokens: 0,
  today_cache_read_tokens: 0,
  today_tokens: 400,
  today_cost: 0.5,
  today_actual_cost: 0.5,
  average_duration_ms: 900,
  rpm: 12,
  tpm: 4000
}

const point = (date: string, overrides: Partial<TrendDataPoint> = {}): TrendDataPoint => ({
  date,
  requests: 0,
  input_tokens: 0,
  output_tokens: 0,
  cache_creation_tokens: 0,
  cache_read_tokens: 0,
  total_tokens: 0,
  cost: 0,
  actual_cost: 0,
  ...overrides
})

const trend = [
  point('2026-10-27', { requests: 10, total_tokens: 100, actual_cost: 0.1 }),
  point('2026-10-28', { requests: 20, total_tokens: 200, actual_cost: 0.2 }),
  point('2026-10-29', { requests: 30, total_tokens: 300, actual_cost: 0.3 })
]

const mountStats = (props: Partial<InstanceType<typeof UserDashboardStats>['$props']> = {}) =>
  mount(UserDashboardStats, {
    props: { stats, balance: 12.5, isSimple: false, ...props },
    global: {
      stubs: {
        RouterLink: { props: ['to'], template: '<a :data-to="to"><slot /></a>' }
      }
    }
  })

describe('UserDashboardStats — sparklines and deltas', () => {
  it('plots a sparkline on every card that has a daily series', () => {
    const wrapper = mountStats({ trend })
    // Requests, cost, today-tokens and the cumulative total-tokens curve.
    expect(wrapper.findAllComponents(SparkLine)).toHaveLength(4)
  })

  it('falls back to plain cards when the trend has not been loaded', () => {
    const wrapper = mountStats()
    expect(wrapper.findAllComponents(SparkLine)).toHaveLength(0)
    expect(wrapper.findAllComponents(StatDelta)).toHaveLength(0)
  })

  it('ignores a one-point trend, which cannot draw a line or a delta', () => {
    const wrapper = mountStats({ trend: [point('2026-10-29', { requests: 5 })] })
    expect(wrapper.findAllComponents(SparkLine)).toHaveLength(0)
  })

  it('shows the delta against the previous day as a percentage', () => {
    const wrapper = mountStats({ trend })
    const deltas = wrapper.findAllComponents(StatDelta)
    // Requests 20 -> 30, cost 0.2 -> 0.3, tokens 200 -> 300: all +50%.
    expect(deltas.map((d) => d.text())).toEqual(['▲50.0%', '▲50.0%', '▲50.0%'])
  })

  it('gives the cumulative total-tokens card a curve but no day-over-day delta', () => {
    const wrapper = mountStats({ trend })
    const cumulative = wrapper
      .findAllComponents(SparkLine)
      .find((spark) => (spark.props('values') as number[]).at(-1) === 600)
    expect(cumulative).toBeDefined()
    // Only three of the four plotted cards carry a delta badge.
    expect(wrapper.findAllComponents(StatDelta)).toHaveLength(3)
  })
})
