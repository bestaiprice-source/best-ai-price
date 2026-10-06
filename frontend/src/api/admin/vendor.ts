import { apiClient } from '../client'

export interface PaymentMethod {
  name: string;
  type: string;
  color?: string;
}

export interface VendorModel {
  id: string;
  vendor_id: string;
  model_name: string;
  input_price: number;
  output_price: number;
  is_available: boolean;
}

export interface Vendor {
  id: string;
  name: string;
  url: string;
  payment_methods: PaymentMethod[];
  currency_type: string;
  exchange_rate: number;
  status: string;
  last_scraped_at: string;
  models?: VendorModel[];
}

export async function getVendors(): Promise<Vendor[]> {
  const { data } = await apiClient.get<Vendor[]>('/admin/vendors');
  return data;
}

export async function addVendor(url: string): Promise<{ id: string }> {
  const { data } = await apiClient.post<{ id: string }>('/admin/vendors', { url });
  return data;
}

export async function scrapeVendor(id: string): Promise<{ message: string, models_count: number }> {
  const { data } = await apiClient.post<{ message: string, models_count: number }>(`/admin/vendors/${id}/scrape`);
  return data;
}

export async function scrapeVendorByUrl(name: string, url: string): Promise<{ message: string, models_count: number }> {
  const { data } = await apiClient.post<{ message: string, models_count: number }>(`/admin/vendors/scrape-by-url`, { name, url });
  return data;
}

export interface VendorPriceHistoryItem {
  id: string;
  vendor_id: string;
  model_name: string;
  input_price: number;
  output_price: number;
  is_available: boolean;
  recorded_at: string;
}

export interface GrossMarginItem {
  name: string;
  requests: number;
  total_tokens: number;
  revenue: number;
  upstream_cost: number;
  gross_profit: number;
  gross_margin_percent: number;
}

export interface GrossMarginAnalytics {
  total_revenue: number;
  total_upstream_cost: number;
  total_gross_profit: number;
  overall_gross_margin_percent: number;
  by_model: GrossMarginItem[];
  by_provider: GrossMarginItem[];
}

export async function getVendorPriceHistory(vendorId?: string, modelName?: string): Promise<VendorPriceHistoryItem[]> {
  const params = new URLSearchParams();
  if (modelName) params.append('model', modelName);
  const url = vendorId ? `/admin/vendors/${vendorId}/price-history?${params.toString()}` : `/admin/vendors/price-history?${params.toString()}`;
  const { data } = await apiClient.get<VendorPriceHistoryItem[]>(url);
  return data;
}

export async function getGrossMarginAnalytics(startDate?: string, endDate?: string): Promise<GrossMarginAnalytics> {
  const params = new URLSearchParams();
  if (startDate) params.append('start_date', startDate);
  if (endDate) params.append('end_date', endDate);
  const { data } = await apiClient.get<GrossMarginAnalytics>(`/admin/usage/gross-margin?${params.toString()}`);
  return data;
}

export interface CheaperProviderOpportunity {
  id: string;
  canonical_model_id: string;
  model_name: string;
  current_primary_provider: string;
  current_primary_price: number;
  current_backup_provider?: string;
  current_backup_price?: number;
  candidate_provider: string;
  candidate_domain: string;
  candidate_price: number;
  candidate_input_price?: number;
  candidate_output_price?: number;
  diff_percent: number;
  billing_unit: string;
  provider_url?: string;
  provider_card_url?: string;
  status: string;
  telegram_sent: boolean;
  discovered_at: string;
  updated_at: string;
}

export async function getProviderOpportunities(status?: string): Promise<CheaperProviderOpportunity[]> {
  const params = new URLSearchParams();
  if (status) params.append('status', status);
  const { data } = await apiClient.get<CheaperProviderOpportunity[]>(`/admin/vendors/opportunities?${params.toString()}`);
  return data;
}

export async function checkProviderOpportunities(): Promise<{ message: string; discovered_count: number; opportunities: CheaperProviderOpportunity[] }> {
  const { data } = await apiClient.post<{ message: string; discovered_count: number; opportunities: CheaperProviderOpportunity[] }>('/admin/vendors/opportunities/check');
  return data;
}

export async function promoteProviderOpportunity(id: string, target: 'primary' | 'backup', groupId?: number, accountId?: number): Promise<void> {
  await apiClient.post(`/admin/vendors/opportunities/${id}/promote`, {
    target,
    group_id: groupId,
    account_id: accountId,
  });
}

export async function dismissProviderOpportunity(id: string): Promise<void> {
  await apiClient.post(`/admin/vendors/opportunities/${id}/dismiss`);
}


