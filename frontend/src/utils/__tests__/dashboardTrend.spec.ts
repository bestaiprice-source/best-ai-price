import { describe, expect, it } from 'vitest'
import {
  cumulativeSeries,
  deltaVsPreviousDay,
  metricSeries,
  type TrendMetric
} from '../dashboardTrend'
import type { TrendDataPoint } from '@/types'

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

describe('metricSeries', () => {
  it('returns the metric values ordered oldest first, regardless of input order', () => {
    const trend = [
      point('2026-10-29', { requests: 30 }),
      point('2026-10-27', { requests: 10 }),
      point('2026-10-28', { requests: 20 })
    ]
    expect(metricSeries(trend, 'requests')).toEqual([10, 20, 30])
  })

  it('coerces non-numeric values to 0 instead of leaking NaN into the chart', () => {
    const trend = [
      point('2026-10-27', { requests: 5 }),
      { ...point('2026-10-28'), requests: undefined as unknown as number }
    ]
    expect(metricSeries(trend, 'requests')).toEqual([5, 0])
  })

  it('returns an empty series for a missing trend', () => {
    expect(metricSeries(undefined, 'requests')).toEqual([])
    expect(metricSeries(null, 'cost')).toEqual([])
  })
})

describe('cumulativeSeries', () => {
  it('accumulates the running total', () => {
    expect(cumulativeSeries([1, 2, 3])).toEqual([1, 3, 6])
  })

  it('returns an empty series for empty input', () => {
    expect(cumulativeSeries([])).toEqual([])
  })
})

describe('deltaVsPreviousDay', () => {
  const metric: TrendMetric = 'actual_cost'

  it('reports an upward move with the percentage against the previous day', () => {
    const trend = [point('2026-10-27', { actual_cost: 2 }), point('2026-10-28', { actual_cost: 3 })]
    const delta = deltaVsPreviousDay(trend, metric)
    expect(delta).not.toBeNull()
    expect(delta?.direction).toBe('up')
    expect(delta?.absolute).toBe(1)
    expect(delta?.percent).toBeCloseTo(50)
  })

  it('reports a downward move as a negative absolute change', () => {
    const trend = [point('2026-10-27', { actual_cost: 4 }), point('2026-10-28', { actual_cost: 3 })]
    const delta = deltaVsPreviousDay(trend, metric)
    expect(delta?.direction).toBe('down')
    expect(delta?.absolute).toBe(-1)
    expect(delta?.percent).toBeCloseTo(-25)
  })

  it('reports a flat move when the value did not change', () => {
    const trend = [point('2026-10-27', { actual_cost: 4 }), point('2026-10-28', { actual_cost: 4 })]
    expect(deltaVsPreviousDay(trend, metric)?.direction).toBe('flat')
  })

  it('leaves the percentage undefined when the previous day was zero', () => {
    const trend = [point('2026-10-27', { actual_cost: 0 }), point('2026-10-28', { actual_cost: 1 })]
    const delta = deltaVsPreviousDay(trend, metric)
    expect(delta?.percent).toBeNull()
    expect(delta?.absolute).toBe(1)
  })

  it('returns null when the trend has fewer than two points', () => {
    expect(deltaVsPreviousDay([point('2026-10-28', { actual_cost: 1 })], metric)).toBeNull()
    expect(deltaVsPreviousDay([], metric)).toBeNull()
    expect(deltaVsPreviousDay(undefined, metric)).toBeNull()
  })
})
