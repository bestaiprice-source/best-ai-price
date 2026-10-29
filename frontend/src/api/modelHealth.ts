import { apiClient } from './client'

/**
 * Model health — `GET /api/v1/model-health`.
 *
 * A flat per-model read model with two sources: channel-monitor probes (an
 * independent request, `source: 'probe'`) and the requests users actually made
 * (`source: 'traffic'`). Probes win whenever both cover a model. A model with
 * neither shows no number at all — never a fake one.
 */
export interface ModelHealthItem {
  /** Upstream id as configured on the monitor, e.g. `claude-3-7-sonnet-20250219`. */
  model: string
  /**
   * Normalization keys produced by the gateway's own model-name resolver. The
   * catalog model id (canonical id or its vendor-less slug) is always one of
   * them, which is how the two feeds are joined without a second normalizer.
   */
  candidates: string[]
  status: string
  availability_pct: number | null
  avg_latency_ms: number | null
  provider: string
  monitor_id: number
  monitor_name: string
  maintenance_until: string | null
  maintenance_note: string
  /** `probe` (measured upstream) or `traffic` (measured on live requests). */
  source?: 'probe' | 'traffic' | string
  /** Traffic sample behind `availability_pct`; absent for probe rows. */
  requests?: number
}

export interface ModelHealthResponse {
  items: ModelHealthItem[]
  window_days: number
}

export async function getModelHealth(windowDays = 7): Promise<ModelHealthResponse> {
  const { data } = await apiClient.get<ModelHealthResponse>('/model-health', {
    params: { window_days: windowDays }
  })
  return {
    items: Array.isArray(data?.items) ? data.items : [],
    window_days: Number(data?.window_days) || windowDays
  }
}
