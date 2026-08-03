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
