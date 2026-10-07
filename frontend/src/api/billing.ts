import { apiClient } from './client'

/**
 * Money-operation ledger (§12) — deposits, API usage debits, refunds and manual
 * adjustments, kept separate from the technical usage log.
 * GET /api/v1/payment/ledger
 */
export type LedgerEntryType = 'deposit' | 'usage' | 'refund' | 'adjustment'

export interface LedgerEntry {
  id: string
  type: LedgerEntryType
  amount_usd: number
  currency: string
  network: string
  status: string
  description: string
  reference: string
  created_at: string
  confirmed_at: string | null
}

export interface BalanceLedgerPage {
  items: LedgerEntry[]
  total: number
  page: number
  page_size: number
  total_deposit_usd: number
  total_usage_usd: number
  total_refund_usd: number
  total_adjustment_usd: number
}

export async function getLedger(params?: {
  type?: string
  page?: number
  page_size?: number
}): Promise<BalanceLedgerPage> {
  const { data } = await apiClient.get<BalanceLedgerPage>('/payment/ledger', { params })
  return data
}
