import { apiClient } from './client'

/**
 * Model health — `GET /api/v1/model-health`.
 *
 * A flat per-model read model over the channel monitors. It is the ONLY source
 * of the uptime percentage shown to users: the public catalog feed carries just
 * a binary availability flag, so percentages have to come from the probes.
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
