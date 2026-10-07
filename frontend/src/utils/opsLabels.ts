// Localized labels for Ops alert metric types.
//
// Alert rule names (`alert_rules.name`) and alert event titles
// (`alert_events.title`) are free-form strings stored in the database and are
// often authored in Chinese. The admin UI must not render those raw strings as
// the primary label in an English interface, so we derive a stable English label
// from the machine-readable `metric_type` enum and keep the stored name as a
// secondary caption.

export const OPS_METRIC_LABELS: Record<string, string> = {
  success_rate: 'Success rate',
  error_rate: 'Error rate',
  upstream_error_rate: 'Upstream error rate',
  cpu_usage_percent: 'CPU usage',
  memory_usage_percent: 'Memory usage',
  concurrency_queue_depth: 'Concurrency queue depth',
  group_available_accounts: 'Group available accounts',
  group_available_ratio: 'Group available ratio',
  group_rate_limit_ratio: 'Group rate-limit ratio',
  account_rate_limited_count: 'Rate-limited accounts',
  account_error_count: 'Account errors',
  account_error_ratio: 'Account error ratio',
  overload_account_count: 'Overloaded accounts'
}

// metricLabel returns a human-readable English label for a metric type, falling
// back to the raw value when unknown.
export function metricLabel(metricType?: string | null): string {
  if (!metricType) return '—'
  return OPS_METRIC_LABELS[metricType] || metricType
}

// isLatinText reports whether a string is predominantly Latin (so we don't show
// a redundant secondary caption when the stored name is already English).
export function isLatinText(value?: string | null): boolean {
  if (!value) return true
  return !/[\u3400-\u9fff\uf900-\ufaff\u3040-\u30ff]/.test(value)
}
