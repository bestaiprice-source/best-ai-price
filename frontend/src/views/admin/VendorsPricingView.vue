<template>
  <div class="vendors-container">
    <div class="header-section">
      <div class="header-content">
        <h2>AI Providers & Pricing</h2>
        <p class="subtitle">Manage unified pricing and payment methods for all upstream API vendors.</p>
      </div>
      <div class="actions">
        <div class="input-group">
          <input v-model="newVendorUrl" placeholder="Enter One-API / New-API URL (e.g. https://yunwu.ai)" class="glass-input" @keyup.enter="handleAddVendor" />
          <button class="primary-btn" @click="handleAddVendor" :disabled="isAdding">
            <span class="icon">➕</span> Add Vendor
          </button>
        </div>
      </div>
    </div>

    <div class="vendors-grid">
      <div v-if="loading" class="loading-state">
        <div class="spinner"></div>
        <p>Fetching latest pricing...</p>
      </div>
      
      <div v-else-if="vendors.length === 0" class="empty-state">
        <div class="empty-icon">🌐</div>
        <h3>No Vendors Configured</h3>
        <p>Add your first vendor URL above to scrape pricing and payment data.</p>
      </div>

      <div v-else v-for="vendor in vendors" :key="vendor.id" class="vendor-card glass-panel">
        <div class="card-header">
          <div class="vendor-info">
            <h3 class="vendor-name">{{ vendor.name }}</h3>
            <a :href="vendor.url" target="_blank" class="vendor-url">{{ vendor.url }}</a>
          </div>
          <button class="sync-btn" @click="handleScrape(vendor.id)" :class="{ 'syncing': syncingId === vendor.id }">
            <span class="icon">🔄</span> {{ syncingId === vendor.id ? 'Syncing...' : 'Sync Prices' }}
          </button>
        </div>

        <div class="card-meta">
          <div class="meta-item">
            <span class="label">Currency:</span>
            <span class="value">{{ vendor.currency_type }} (1 USD = {{ vendor.exchange_rate }})</span>
          </div>
          <div class="meta-item">
            <span class="label">Last Updated:</span>
            <span class="value">{{ formatDate(vendor.last_scraped_at) }}</span>
          </div>
        </div>

        <div class="payment-methods">
          <span class="label">Accepted Payments:</span>
          <div class="tags-container">
            <span v-for="method in vendor.payment_methods" :key="method.type" 
                  class="payment-tag" :style="{ backgroundColor: method.color || '#333' }">
              {{ method.name }}
            </span>
            <span v-if="!vendor.payment_methods || vendor.payment_methods.length === 0" class="text-muted">None found</span>
          </div>
        </div>

        <div class="pricing-section">
          <div class="pricing-header" @click="toggleModels(vendor.id)">
            <h4>Supported Models ({{ vendor.models?.length || 0 }})</h4>
            <span class="toggle-icon" :class="{ 'open': expandedVendorId === vendor.id }">▼</span>
          </div>
          
          <div class="models-table-container" v-show="expandedVendorId === vendor.id">
            <table class="models-table">
              <thead>
                <tr>
                  <th>Model Name</th>
                  <th>Input Price (Base Multiplier)</th>
                  <th>Output Price (Base Multiplier)</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="model in vendor.models" :key="model.model_name">
                  <td class="fw-bold">{{ model.model_name }}</td>
                  <td class="price-val">${{ model.input_price.toFixed(6) }}</td>
                  <td class="price-val">${{ model.output_price.toFixed(6) }}</td>
                  <td>
                    <span class="status-badge" :class="model.is_available ? 'active' : 'inactive'">
                      {{ model.is_available ? 'Available' : 'Unavailable' }}
                    </span>
                  </td>
                </tr>
                <tr v-if="!vendor.models || vendor.models.length === 0">
                  <td colspan="4" class="text-center text-muted">No models synced yet. Click Sync Prices.</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { getVendors, addVendor, scrapeVendor, Vendor } from '@/api/admin/vendor';
import { useAppStore } from '@/stores'

const appStore = useAppStore();
const vendors = ref<Vendor[]>([]);
const loading = ref(true);
const isAdding = ref(false);
const newVendorUrl = ref('');
const syncingId = ref<string | null>(null);
const expandedVendorId = ref<string | null>(null);

const loadVendors = async () => {
  try {
    loading.value = true;
    const res = await getVendors();
    vendors.value = res;
  } catch (error: any) {
    appStore.showError(error.message || 'Failed to load vendors');
  } finally {
    loading.value = false;
  }
};

const handleAddVendor = async () => {
  if (!newVendorUrl.value) return;
  try {
    isAdding.value = true;
    await addVendor(newVendorUrl.value);
    appStore.showSuccess('Vendor added successfully');
    newVendorUrl.value = '';
    await loadVendors();
  } catch (error: any) {
    appStore.showError(error.message || 'Failed to add vendor');
  } finally {
    isAdding.value = false;
  }
};

const handleScrape = async (id: string) => {
  try {
    syncingId.value = id;
    const res = await scrapeVendor(id);
    appStore.showSuccess(res.message + ` (${res.models_count} models synced)`);
    await loadVendors();
    expandedVendorId.value = id; // Auto expand after sync
  } catch (error: any) {
    appStore.showError(error.message || 'Scrape failed');
  } finally {
    syncingId.value = null;
  }
};

const toggleModels = (id: string) => {
  if (expandedVendorId.value === id) {
    expandedVendorId.value = null;
  } else {
    expandedVendorId.value = id;
  }
};

const formatDate = (dateStr: string) => {
  if (!dateStr) return 'Never';
  return new Date(dateStr).toLocaleString();
};

onMounted(() => {
  loadVendors();
});
</script>

<style scoped>
.vendors-container {
  padding: 24px;
  max-width: 1200px;
  margin: 0 auto;
  color: var(--text-primary, #e0e0e0);
  font-family: 'Inter', sans-serif;
}

.header-section {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 32px;
  padding-bottom: 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.header-content h2 {
  font-size: 24px;
  font-weight: 700;
  margin: 0 0 8px 0;
  background: linear-gradient(90deg, #fff, #a5b4fc);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

.subtitle {
  color: #94a3b8;
  margin: 0;
  font-size: 14px;
}

.input-group {
  display: flex;
  gap: 12px;
}

.glass-input {
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
  padding: 10px 16px;
  border-radius: 8px;
  color: #fff;
  width: 300px;
  transition: all 0.3s ease;
}

.glass-input:focus {
  outline: none;
  border-color: #6366f1;
  box-shadow: 0 0 0 2px rgba(99, 102, 241, 0.2);
}

.primary-btn {
  background: #6366f1;
  color: white;
  border: none;
  padding: 10px 20px;
  border-radius: 8px;
  cursor: pointer;
  font-weight: 600;
  transition: all 0.2s;
  display: flex;
  align-items: center;
  gap: 8px;
}

.primary-btn:hover:not(:disabled) {
  background: #4f46e5;
  transform: translateY(-1px);
}

.primary-btn:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.glass-panel {
  background: rgba(30, 41, 59, 0.7);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 16px;
  padding: 24px;
  margin-bottom: 24px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.2);
  transition: transform 0.3s ease, box-shadow 0.3s ease;
}

.glass-panel:hover {
  transform: translateY(-2px);
  box-shadow: 0 12px 48px rgba(0, 0, 0, 0.3);
  border-color: rgba(255, 255, 255, 0.12);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 16px;
}

.vendor-name {
  font-size: 20px;
  font-weight: 600;
  margin: 0 0 4px 0;
  color: #fff;
}

.vendor-url {
  color: #6366f1;
  text-decoration: none;
  font-size: 13px;
}

.vendor-url:hover {
  text-decoration: underline;
}

.sync-btn {
  background: rgba(255, 255, 255, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.2);
  color: #fff;
  padding: 8px 16px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
}

.sync-btn:hover {
  background: rgba(255, 255, 255, 0.15);
}

.syncing .icon {
  display: inline-block;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  100% { transform: rotate(360deg); }
}

.card-meta {
  display: flex;
  gap: 24px;
  margin-bottom: 20px;
  padding-bottom: 20px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.meta-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.label {
  font-size: 12px;
  color: #94a3b8;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.value {
  font-size: 14px;
  color: #e2e8f0;
  font-weight: 500;
}

.payment-methods {
  margin-bottom: 24px;
}

.tags-container {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 8px;
}

.payment-tag {
  padding: 4px 12px;
  border-radius: 20px;
  font-size: 12px;
  font-weight: 600;
  color: #fff;
  text-shadow: 0 1px 2px rgba(0,0,0,0.2);
  box-shadow: 0 2px 4px rgba(0,0,0,0.1);
}

.pricing-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  cursor: pointer;
  padding: 12px;
  background: rgba(0, 0, 0, 0.2);
  border-radius: 8px;
  transition: background 0.2s;
}

.pricing-header:hover {
  background: rgba(0, 0, 0, 0.3);
}

.pricing-header h4 {
  margin: 0;
  font-size: 15px;
  color: #e2e8f0;
}

.toggle-icon {
  transition: transform 0.3s;
  font-size: 12px;
}

.toggle-icon.open {
  transform: rotate(180deg);
}

.models-table-container {
  margin-top: 12px;
  animation: slideDown 0.3s ease-out;
}

@keyframes slideDown {
  from { opacity: 0; transform: translateY(-10px); }
  to { opacity: 1; transform: translateY(0); }
}

.models-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.models-table th, .models-table td {
  padding: 12px;
  text-align: left;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.models-table th {
  color: #94a3b8;
  font-weight: 600;
  background: rgba(0, 0, 0, 0.1);
}

.fw-bold {
  font-weight: 600;
  color: #fff;
}

.price-val {
  font-family: monospace;
  color: #a7f3d0;
}

.status-badge {
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 600;
}

.status-badge.active {
  background: rgba(16, 185, 129, 0.2);
  color: #34d399;
}

.status-badge.inactive {
  background: rgba(239, 68, 68, 0.2);
  color: #f87171;
}

.text-center { text-align: center; }
.text-muted { color: #64748b; font-size: 13px; }

/* Loading State */
.loading-state, .empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 0;
  color: #94a3b8;
}

.spinner {
  width: 40px;
  height: 40px;
  border: 3px solid rgba(99, 102, 241, 0.2);
  border-top-color: #6366f1;
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin-bottom: 16px;
}

.empty-icon {
  font-size: 48px;
  margin-bottom: 16px;
  opacity: 0.5;
}
</style>
