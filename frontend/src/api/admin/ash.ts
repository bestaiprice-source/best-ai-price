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
