/**
 * Dashboard trend helpers — sparkline series and "vs yesterday" deltas.
 *
 * The user dashboard already loads a 7-day daily trend
 * (`usageAPI.getDashboardTrend`), so the stat cards derive both their sparkline
 * and their delta from that single response instead of issuing another request.
 *
 * Metrics without a daily series deliberately get neither: the trend query only
 * aggregates requests / tokens / cost per day, so balance, API-key count, the
 * RPM gauge and the average response time have nothing honest to plot.
 */
import type { TrendDataPoint } from '@/types'

/** Daily metrics the user trend endpoint actually aggregates. */
export type TrendMetric =
  | 'requests'
  | 'total_tokens'
  | 'cost'
  | 'actual_cost'
  | 'input_tokens'
  | 'output_tokens'
  | 'cache_read_tokens'
  | 'cache_creation_tokens'

export interface TrendDelta {
  direction: 'up' | 'down' | 'flat'
  /** Percentage change vs the previous day; null when the previous day is 0. */
  percent: number | null
  /** Absolute change (current - previous). */
  absolute: number
  current: number
  previous: number
}

const sortedPoints = (trend: TrendDataPoint[] | null | undefined): TrendDataPoint[] =>
  [...(trend ?? [])].sort((a, b) => (a.date < b.date ? -1 : a.date > b.date ? 1 : 0))

const valueOf = (point: TrendDataPoint, metric: TrendMetric): number => {
  const value = Number((point as unknown as Record<string, unknown>)[metric])
  return Number.isFinite(value) ? value : 0
}

/** Ordered daily values for one metric (oldest first). */
export function metricSeries(
  trend: TrendDataPoint[] | null | undefined,
  metric: TrendMetric
): number[] {
  return sortedPoints(trend).map((point) => valueOf(point, metric))
}

/** Running total of a series — used by the cumulative "total tokens" card. */
export function cumulativeSeries(values: number[]): number[] {
  let sum = 0
  return values.map((value) => (sum += value))
}

/**
 * Change of the latest day against the one before it ("vs yesterday").
 * Returns null when the trend carries fewer than two points.
 */
export function deltaVsPreviousDay(
  trend: TrendDataPoint[] | null | undefined,
  metric: TrendMetric
): TrendDelta | null {
  const series = metricSeries(trend, metric)
  if (series.length < 2) return null
  const current = series[series.length - 1]
  const previous = series[series.length - 2]
  const absolute = current - previous
  const direction = absolute > 0 ? 'up' : absolute < 0 ? 'down' : 'flat'
  return {
    direction,
    percent: previous > 0 ? (absolute / previous) * 100 : null,
    absolute,
    current,
    previous
  }
}
