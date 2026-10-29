import { apiClient } from '../client'

export interface ProviderMetrics {
  latency_ms: number
  error_rate: number
  requests: number
  spend_usd: number
}

export type ProviderState = 'Healthy' | 'Degraded' | 'Down' | 'Invalid key' | 'Low balance' | 'Disabled'

export type ProviderProtocol = 'openai' | 'anthropic' | 'gemini' | 'responses'

export interface Provider {
  id: number
  provider_name: string
  provider_type: string
  domain: string
  base_url: string
  protocol: ProviderProtocol
  api_key_masked: string
  has_api_key: boolean
  supported_model_families: string[]
  model_mapping: Record<string, string>
  balance_usd: number
  balance_source: string
  last_synced_at: string | null
  min_balance_usd: number
  state: ProviderState
  metrics: ProviderMetrics
  status: 'active' | 'disabled'
  created_at: string
  updated_at: string
}

export interface ProviderSummaryStats {
  total_providers: number
  active_providers: number
  low_balance_providers: number
  total_balance_usd: number
  total_spend_usd: number
}

export interface ProviderListResponse {
  items?: Provider[]
  providers?: Provider[]
  total: number
  page: number
  page_size: number
  summary: ProviderSummaryStats
}

export interface ProviderListFilters {
  search?: string
  status?: string
  state?: string
}

export interface CreateProviderRequest {
  provider_name: string
  provider_type?: string
  domain?: string
  base_url?: string
  protocol?: ProviderProtocol
  api_key: string
  supported_model_families?: string[]
  model_mapping?: Record<string, string>
  balance_usd?: number
  min_balance_usd?: number
  status?: 'active' | 'disabled'
}

export interface UpdateProviderRequest {
  provider_name?: string
  provider_type?: string
  domain?: string
  base_url?: string
  protocol?: ProviderProtocol
  api_key?: string
  supported_model_families?: string[]
  model_mapping?: Record<string, string>
  balance_usd?: number
  min_balance_usd?: number
  status?: 'active' | 'disabled'
}

export interface TestConnectionResponse {
  success: boolean
  latency_ms: number
  status_code: number
  model_count: number
  error?: string
}

export interface SyncBalanceResponse {
  id: number
  balance_usd: number
  total_granted: number
  total_used: number
  source: string
  last_synced_at: string
}

export interface BatchSyncSummary {
  total: number
  successful: number
  failed: number
  errors?: Array<{
    id: number
    name: string
    error: string
  }>
}

/**
 * Fetch paginated list of providers with summary metrics
 */
export async function getProviders(
  page: number = 1,
  pageSize: number = 20,
  filters?: ProviderListFilters
): Promise<ProviderListResponse> {
  const { data } = await apiClient.get<ProviderListResponse>('/admin/providers', {
    params: {
      page,
      page_size: pageSize,
      ...filters
    }
  })
  return data
}

/**
 * Get provider details by ID
 */
export async function getProvider(id: number): Promise<Provider> {
  const { data } = await apiClient.get<Provider>(`/admin/providers/${id}`)
  return data
}

/**
 * Create a new provider
 */
export async function createProvider(req: CreateProviderRequest): Promise<Provider> {
  const { data } = await apiClient.post<Provider>('/admin/providers', req)
  return data
}

/**
 * Update an existing provider
 */
export async function updateProvider(id: number, req: UpdateProviderRequest): Promise<Provider> {
  const { data } = await apiClient.put<Provider>(`/admin/providers/${id}`, req)
  return data
}

/**
 * Delete a provider
 */
export async function deleteProvider(id: number): Promise<{ id: number }> {
  const { data } = await apiClient.delete<{ id: number }>(`/admin/providers/${id}`)
  return data
}

/**
 * Test connectivity and latency for an existing provider
 */
export async function testProviderConnection(id: number): Promise<TestConnectionResponse> {
  const { data } = await apiClient.post<TestConnectionResponse>(`/admin/providers/${id}/test`)
  return data
}

/**
 * Test connectivity and latency for draft credentials
 */
export async function testDraftProviderConnection(req: {
  base_url?: string
  protocol?: string
  api_key: string
}): Promise<TestConnectionResponse> {
  const { data } = await apiClient.post<TestConnectionResponse>('/admin/providers/test-draft', req)
  return data
}

/**
 * Synchronize real-time balance for a single provider
 */
export async function syncProviderBalance(id: number): Promise<SyncBalanceResponse> {
  const { data } = await apiClient.post<SyncBalanceResponse>(`/admin/providers/${id}/sync-balance`)
  return data
}

/**
 * Batch synchronize balances across all active providers
 */
export async function syncAllProviderBalances(): Promise<BatchSyncSummary> {
  const { data } = await apiClient.post<BatchSyncSummary>('/admin/providers/sync-all')
  return data
}

/**
 * Resolved Primary/Backup routing table (§6): for every sellable canonical model,
 * the two providers that will serve it, their upstream model ids, the applied
 * retail price, route status and when both routes were last verified.
 */
export interface ModelRouteProvider {
  account_id: number
  provider: string
  name: string
  domain: string
  upstream_model: string
  available: boolean
  last_success_at?: string
}

export interface ModelRouteItem {
  canonical_model_id: string
  primary: ModelRouteProvider | null
  backup: ModelRouteProvider | null
  retail_input_per_1m: number | null
  retail_output_per_1m: number | null
  status: 'available' | 'degraded' | 'unavailable'
  route_source: 'explicit' | 'price'
  last_verified?: string
}

export interface ModelRoutesResponse {
  group_id?: number
  items: ModelRouteItem[]
  total: number
}

export async function getModelRoutes(filters?: { group_id?: number; model?: string }): Promise<ModelRoutesResponse> {
  const { data } = await apiClient.get<ModelRoutesResponse>('/admin/model-routes', { params: filters })
  return data
}

/**
 * The billing groups a provider's price list exposes — the dropdown source for
 * the key-group fields of an account.
 */
export interface ProviderBillingGroup {
  name: string
  ratio: number
  models: number
}

export interface ProviderBillingGroupsResponse {
  provider: string
  groups: ProviderBillingGroup[]
}

export async function getBillingGroups(params: {
  provider?: string
  base_url?: string
}): Promise<ProviderBillingGroupsResponse> {
  const { data } = await apiClient.get<ProviderBillingGroupsResponse>('/admin/providers/groups', { params })
  return data
}
