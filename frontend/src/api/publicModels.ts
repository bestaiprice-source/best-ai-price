import { apiClient } from './client'

/**
 * Public model catalog pricing — `GET /models` (BestAIPrice gateway format).
 *
 * §11.3 needs a per-model availability status next to the retail price, and this
 * is the same public feed the catalog/BestAIPrice row uses, so the status shown
 * to users can never diverge from what the gateway actually serves.
 */
export interface PublicModelPricing {
  canonical_model_id?: string
  model_id?: string
  available?: boolean
  status?: string
  /** Retail (charged) prices, USD per 1M tokens — already include the markup. */
  input_price_per_1m?: number | null
  output_price_per_1m?: number | null
  cache_read_price_per_1m?: number | null
  cache_write_price_per_1m?: number | null
  /** Flat-rate / per-unit price for media models (image, video, audio). */
  native_price_usd?: number | null
  native_price_unit?: string | null
  /** Vendor list price and how far below it we sell (catalog badges). */
  official_input_price_per_1m?: number | null
  official_output_price_per_1m?: number | null
  discount_percent?: number | null
}

export async function getPublicModels(): Promise<PublicModelPricing[]> {
  const { data } = await apiClient.get<PublicModelPricing[]>('/models')
  return Array.isArray(data) ? data : []
}
