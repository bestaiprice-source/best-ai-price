import { apiClient } from '../client'

export interface RunStatsTotals {
  models: number
  providers: number
  auth_providers: number
  offers: number
}

export interface RunStatsCurrentRun {
  status: string
  started_at: string
  updated_at: string
  providers_done: number
  providers_total: number
  prices_extracted: number
  errors_count: number
}

export interface RunStatsFullRun {
  started_at: string
  finished_at: string
  duration_seconds: number
  success: boolean
  providers_found: number
  providers_unique: number
  pricing_pages_found: number
  prices_extracted: number
  prices_published: number
  records_needing_review: number
  errors_count: number
}

export interface RunStatsNewModel {
  canonical_model_id: string
  display_name: string
  providers: string[]
  via_auth_provider: boolean
}

export interface RunStatsNewProvider {
  provider_domain: string
  provider_name: string
  requires_auth: boolean
}

export interface RunStatsChange {
  at: string | null
  baseline: boolean
  in_progress: boolean
  new_models: RunStatsNewModel[]
  new_providers: RunStatsNewProvider[]
  removed_models: string[]
  removed_providers: string[]
  new_offers_count: number
}

export interface RunStatsSnapshot {
  models: string[]
  providers: string[]
  offers: [string, string][]
}

export interface RunStatsData {
  last_sync_at: string | null
  totals: RunStatsTotals
  last_change: RunStatsChange | null
  history: RunStatsChange[]
  current_run: RunStatsCurrentRun | null
  last_full_run: RunStatsFullRun | null
  snapshot: RunStatsSnapshot | null
  source_path?: string
  file_exists: boolean
  file_mod_time?: string
  candidate_paths?: string[]
  checked_errors?: Record<string, string>
}

export interface ParserStatsApiResponse {
  code: number
  data: RunStatsData
  message?: string
}

export async function getParserRunStats(): Promise<RunStatsData> {
  const res = await apiClient.get<ParserStatsApiResponse>('/api/v1/admin/parser/run-stats')
  return res.data.data
}
