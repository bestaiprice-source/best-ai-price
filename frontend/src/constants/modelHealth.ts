/**
 * Model-health status vocabulary.
 *
 * Single source of truth for the OpenLux-style legend every screen uses
 * (dashboard health block, model catalog cards, channel-status screen):
 * Normal / High latency / Error / Maintenance.
 *
 * The wire values come from the channel monitor (`operational` / `degraded` /
 * `failed` / `error`) plus the monitor-level `maintenance` overlay; the user
 * never sees those raw strings.
 */

export type ModelHealthStatus =
  | 'operational'
  | 'degraded'
  | 'failed'
  | 'error'
  | 'maintenance'
  | 'unknown'

export interface ModelHealthStatusMeta {
  id: ModelHealthStatus
  /** i18n key of the user-facing label (OpenLux wording, not the raw enum). */
  labelKey: string
  dotClass: string
  textClass: string
  badgeClass: string
  /**
   * Healthiness rank — lower is better. Used to pick the best status when
   * several monitors cover the same model: the model is sellable as long as ONE
   * monitored provider serves it, and we route to the cheapest healthy one, so
   * a single failing provider must not paint the model red.
   */
  rank: number
}

export const MODEL_HEALTH_STATUSES: readonly ModelHealthStatusMeta[] = [
  {
    id: 'operational',
    labelKey: 'modelHealth.status.normal',
    dotClass: 'bg-emerald-500',
    textClass: 'text-emerald-600 dark:text-emerald-400',
    badgeClass: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
    rank: 0
  },
  {
    id: 'degraded',
    labelKey: 'modelHealth.status.highLatency',
    dotClass: 'bg-amber-500',
    textClass: 'text-amber-600 dark:text-amber-400',
    badgeClass: 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
    rank: 1
  },
  {
    id: 'maintenance',
    labelKey: 'modelHealth.status.maintenance',
    dotClass: 'bg-sky-500',
    textClass: 'text-sky-600 dark:text-sky-400',
    badgeClass: 'bg-sky-50 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300',
    rank: 2
  },
  {
    id: 'error',
    labelKey: 'modelHealth.status.error',
    dotClass: 'bg-rose-500',
    textClass: 'text-rose-600 dark:text-rose-400',
    badgeClass: 'bg-rose-50 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300',
    rank: 3
  },
  {
    id: 'failed',
    labelKey: 'modelHealth.status.error',
    dotClass: 'bg-rose-500',
    textClass: 'text-rose-600 dark:text-rose-400',
    badgeClass: 'bg-rose-50 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300',
    rank: 3
  },
  {
    id: 'unknown',
    labelKey: 'modelHealth.status.unknown',
    dotClass: 'bg-gray-300',
    textClass: 'text-gray-400 dark:text-gray-500',
    badgeClass: 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400',
    rank: 4
  }
]

const BY_ID = new Map<string, ModelHealthStatusMeta>(
  MODEL_HEALTH_STATUSES.map((meta) => [meta.id, meta])
)

/** Resolves any wire status onto its display metadata (unknown ⇒ gray). */
export function modelHealthStatusMeta(status: string | null | undefined): ModelHealthStatusMeta {
  const key = (status ?? '').trim().toLowerCase()
  return BY_ID.get(key) ?? BY_ID.get('unknown')!
}

/** The four entries the legend renders (failed shares the Error entry). */
export const MODEL_HEALTH_LEGEND: readonly ModelHealthStatusMeta[] = [
  BY_ID.get('operational')!,
  BY_ID.get('degraded')!,
  BY_ID.get('error')!,
  BY_ID.get('maintenance')!
]
