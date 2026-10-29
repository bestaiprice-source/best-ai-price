import { apiClient } from './client'

/**
 * API speed test — `POST /api/v1/usage/speed-test`.
 *
 * Runs ONE minimal real completion (max_tokens 1) for the requested model
 * through the same engine path as normal traffic and reports how long it took.
 * It is billed like any other request, so the number is the latency a user
 * actually gets rather than a synthetic ping.
 */
export interface SpeedTestResult {
  model: string
  /** Time to first token, ms. */
  ttft_ms: number
  /** Whole request, ms. */
  total_ms: number
  input_tokens: number
  output_tokens: number
  node_id: string
}

export async function runSpeedTest(model: string): Promise<SpeedTestResult> {
  const { data } = await apiClient.post<SpeedTestResult>('/usage/speed-test', { model })
  return data
}
