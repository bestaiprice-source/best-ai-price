/**
 * Hypervisor (ASH) Admin API Client
 * Provides endpoints for monitoring system state, node health, and manual node control.
 */

import { apiClient } from '../client'

export interface AshHealthResponse {
  status: string
  system_state: string
  platform: string
  total_nodes: number
  healthy_nodes: number
  cooldown_nodes: number
}

export interface AshGlobalStats {
  total_input_tokens: number
  total_output_tokens: number
  total_cost_units: number
  active_sessions_count: number
}

export interface AshNodeMetric {
  node_id: string
  account_name: string
  provider: string
  state: string
  capabilities: string[]
  success_rate: number
  avg_latency_ms: number
  recent_429: number
  recent_timeouts: number
  cooldown_until?: string
}

export async function getAshHealth(platform = 'anthropic'): Promise<AshHealthResponse> {
  const { data } = await apiClient.get<AshHealthResponse>(`/ash/health?platform=${platform}`)
  return data
}

export async function getAshGlobalStats(): Promise<AshGlobalStats> {
  const { data } = await apiClient.get<AshGlobalStats>('/ash/stats')
  return data
}

export async function getAshNodeMetrics(): Promise<AshNodeMetric[]> {
  const { data } = await apiClient.get<AshNodeMetric[]>('/ash/stats/nodes')
  return data
}

export async function setAshNodeCooldown(nodeID: string): Promise<{ status: string; cooldown_until: string }> {
  const { data } = await apiClient.post<{ status: string; cooldown_until: string }>(`/ash/admin/nodes/${nodeID}/cooldown`)
  return data
}

export async function enableAshNode(nodeID: string): Promise<{ status: string; node_state: string }> {
  const { data } = await apiClient.post<{ status: string; node_state: string }>(`/ash/admin/nodes/${nodeID}/enable`)
  return data
}

export async function disableAshNode(nodeID: string): Promise<{ status: string; node_state: string }> {
  const { data } = await apiClient.post<{ status: string; node_state: string }>(`/ash/admin/nodes/${nodeID}/disable`)
  return data
}

export interface AshAnalytics {
  rebinding_24h_count: number
  total_events_24h: number
  pool_utilization_percent: number
}

export interface AshDepositItem {
  id: number
  owner_id: string
  amount_cents: number
  currency: string
  state: string
  notice_days: number
  created_at: string
  released_at?: string
}

export async function getAshAnalytics(): Promise<AshAnalytics> {
  const { data } = await apiClient.get<AshAnalytics>('/ash/stats/analytics')
  return data
}

export async function getAshDeposits(ownerID?: string): Promise<AshDepositItem[]> {
  const url = ownerID ? `/ash/admin/deposits?owner_id=${ownerID}` : '/ash/admin/deposits'
  const { data } = await apiClient.get<AshDepositItem[]>(url)
  return data
}

export async function createAshDeposit(payload: { owner_id: string; amount_cents: number; currency?: string; notice_days?: number }): Promise<AshDepositItem> {
  const { data } = await apiClient.post<AshDepositItem>('/ash/admin/deposits', payload)
  return data
}

export async function releaseAshDeposit(depositID: number, ownerID?: string): Promise<{ status: string; deposit_state: string }> {
  const url = ownerID ? `/ash/admin/deposits/${depositID}/release?owner_id=${ownerID}` : `/ash/admin/deposits/${depositID}/release`
  const { data } = await apiClient.post<{ status: string; deposit_state: string }>(url)
  return data
}

export interface YunwuNodeItem {
  node_id: string
  account_name: string
  platform: string
  type: string
  state: string
  total_available_usd: number
  base_url?: string
  status: string
}

export async function getAshYunwuHealth(): Promise<AshHealthResponse> {
  const { data } = await apiClient.get<AshHealthResponse>('/ash/admin/yunwu/health')
  return data
}

export async function getAshYunwuNodes(): Promise<YunwuNodeItem[]> {
  const { data } = await apiClient.get<YunwuNodeItem[]>('/ash/admin/yunwu/nodes')
  return data
}

export async function syncAshYunwuNode(nodeID: string, apiKey?: string, baseURL?: string): Promise<{ total_available: number; total_used: number }> {
  const { data } = await apiClient.post<{ total_available: number; total_used: number }>(`/ash/admin/yunwu/nodes/${nodeID}/sync`, {
    api_key: apiKey,
    base_url: baseURL
  })
  return data
}
