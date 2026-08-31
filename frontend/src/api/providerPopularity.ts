import apiClient from './client'

export interface ProviderPopularityItem {
  provider: string
  click_count: number
  rank?: number
  last_clicked_at?: string
}

export interface RecordClickResult {
  provider: string
  click_count: number
  counted: boolean
  cooldown?: boolean
}

export const providerPopularityAPI = {
  /** Record a click on a provider with anti-abuse verification */
  recordClick(provider: string) {
    return apiClient.post<RecordClickResult>(`/providers/${encodeURIComponent(provider)}/click`)
  },

  /** Get popularity rankings for all providers */
  getPopularity() {
    return apiClient.get<{ providers: ProviderPopularityItem[] }>('/providers/popularity')
  },

  /** Get popularity stats for a specific provider */
  getProviderPopularity(provider: string) {
    return apiClient.get<ProviderPopularityItem>(`/providers/${encodeURIComponent(provider)}/popularity`)
  }
}

export default providerPopularityAPI
