<template>
  <AppLayout>
    <div class="vendors-container">
      <div class="header-section">
        <div class="header-content">
          <h2>AI Providers & Pricing</h2>
          <p class="subtitle">Manage unified pricing and payment methods for all upstream API vendors.</p>
        </div>
        <div class="actions">
          <button class="primary-btn" @click="handleSyncAll" :disabled="isSyncingAll">
            <span class="icon">🔄</span> {{ isSyncingAll ? 'Syncing...' : 'Sync All Prices' }}
          </button>
        </div>
      </div>

      <div class="table-container glass-panel">
        <table class="vendors-table">
          <thead>
            <tr>
              <th>Provider</th>
              <th>URL</th>
              <th>Payment Methods</th>
              <th>Models Count</th>
              <th>Last Updated</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="vendor in displayVendors" :key="vendor.url">
              <td class="fw-bold">{{ vendor.name }}</td>
              <td><a :href="vendor.url" target="_blank">{{ vendor.url }}</a></td>
              <td>
                <div class="tags-container">
                  <span v-for="method in vendor.payment_methods" :key="method.type" 
                        class="payment-tag" :style="{ backgroundColor: method.color || '#333' }">
                    {{ method.name }}
                  </span>
                  <span v-if="!vendor.payment_methods || vendor.payment_methods.length === 0" class="text-muted">None</span>
                </div>
              </td>
              <td>{{ vendor.models?.length || 0 }}</td>
              <td>{{ formatDate(vendor.last_scraped_at) }}</td>
              <td>
                <div class="action-buttons">
                  <button class="action-btn sync-btn" @click="handleScrape(vendor)" :disabled="syncingId === vendor.url">
                    <span class="icon">🔄</span> {{ syncingId === vendor.url ? 'Syncing...' : 'Sync' }}
                  </button>
                  <button class="action-btn view-btn" @click="openModelsModal(vendor)" :disabled="!vendor.models || vendor.models.length === 0">
                    <span class="icon">👁️</span> View Models
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Models Modal -->
    <div v-if="selectedVendor" class="modal-overlay" @click.self="closeModelsModal">
      <div class="modal-content glass-panel">
        <div class="modal-header">
          <h3>Models for {{ selectedVendor.name }}</h3>
          <button class="close-btn" @click="closeModelsModal">✕</button>
        </div>
        <div class="modal-body">
          <table class="models-table">
            <thead>
              <tr>
                <th>Model Name</th>
                <th>Input Price</th>
                <th>Output Price</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="model in selectedVendor.models" :key="model.model_name">
                <td class="fw-bold">{{ model.model_name }}</td>
                <td class="price-val">${{ model.input_price.toFixed(6) }}</td>
                <td class="price-val">${{ model.output_price.toFixed(6) }}</td>
                <td>
                  <span class="status-badge" :class="model.is_available ? 'active' : 'inactive'">
                    {{ model.is_available ? 'Available' : 'Unavailable' }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import AppLayout from '@/components/layout/AppLayout.vue';
import { getVendors, scrapeVendorByUrl, Vendor } from '@/api/admin/vendor';
import { useAppStore } from '@/stores'

const appStore = useAppStore();
const dbVendors = ref<Vendor[]>([]);
const loading = ref(true);
const syncingId = ref<string | null>(null);
const isSyncingAll = ref(false);
const selectedVendor = ref<Vendor | null>(null);

const HARDCODED_VENDORS = [
  { name: 'Yunwu', url: 'https://api.openlux.ai' }
];

const displayVendors = computed(() => {
  return HARDCODED_VENDORS.map(hc => {
    const dbMatch = dbVendors.value.find(dbv => dbv.url === hc.url);
    if (dbMatch) {
      return dbMatch;
    }
    // Return mock un-synced vendor
    return {
      id: '',
      name: hc.name,
      url: hc.url,
      payment_methods: [],
      currency_type: 'USD',
      exchange_rate: 1.0,
      status: 'pending',
      last_scraped_at: '',
      models: []
    } as Vendor;
  });
});

const loadVendors = async () => {
  try {
    loading.value = true;
    const res = await getVendors();
    dbVendors.value = res || [];
  } catch (error: any) {
    appStore.showError(error.message || 'Failed to load vendors');
  } finally {
    loading.value = false;
  }
};

const handleScrape = async (vendor: Vendor) => {
  try {
    syncingId.value = vendor.url;
    const res = await scrapeVendorByUrl(vendor.name, vendor.url);
    appStore.showSuccess(res.message + ` (${res.models_count} models synced)`);
    await loadVendors();
  } catch (error: any) {
    appStore.showError(error.message || 'Scrape failed');
  } finally {
    syncingId.value = null;
  }
};

const handleSyncAll = async () => {
  try {
    isSyncingAll.value = true;
    for (const vendor of displayVendors.value) {
      await scrapeVendorByUrl(vendor.name, vendor.url);
    }
    appStore.showSuccess('All vendors synced successfully');
    await loadVendors();
  } catch (error: any) {
    appStore.showError(error.message || 'Sync all failed');
  } finally {
    isSyncingAll.value = false;
  }
};

const openModelsModal = (vendor: Vendor) => {
  selectedVendor.value = vendor;
};

const closeModelsModal = () => {
  selectedVendor.value = null;
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
  max-width: 1400px;
  margin: 0 auto;
  color: var(--text-primary, #e0e0e0);
  font-family: 'Inter', sans-serif;
}

.header-section {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 32px;
}

.header-content h2 {
  font-size: 28px;
  font-weight: 700;
  margin: 0 0 8px 0;
  background: linear-gradient(90deg, #fff, #a5b4fc);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

.subtitle {
  color: #9ca3af;
  margin: 0;
  font-size: 15px;
}

.primary-btn {
  background: linear-gradient(135deg, #6366f1 0%, #4f46e5 100%);
  color: white;
  border: none;
  padding: 10px 20px;
  border-radius: 8px;
  font-weight: 600;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 8px;
  transition: all 0.2s ease;
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.3);
}

.primary-btn:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 6px 16px rgba(99, 102, 241, 0.4);
}

.primary-btn:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.glass-panel {
  background: rgba(30, 41, 59, 0.7);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 12px;
}

.table-container {
  overflow-x: auto;
  border-radius: 12px;
}

.vendors-table, .models-table {
  width: 100%;
  border-collapse: collapse;
}

.vendors-table th, .models-table th {
  background: rgba(15, 23, 42, 0.6);
  padding: 16px;
  text-align: left;
  font-weight: 600;
  color: #94a3b8;
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.vendors-table td, .models-table td {
  padding: 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  font-size: 14px;
}

.vendors-table tr:hover, .models-table tr:hover {
  background: rgba(255, 255, 255, 0.02);
}

.fw-bold {
  font-weight: 600;
  color: #f1f5f9;
}

.text-muted {
  color: #64748b;
}

a {
  color: #60a5fa;
  text-decoration: none;
}
a:hover {
  text-decoration: underline;
}

.tags-container {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.payment-tag {
  padding: 4px 10px;
  border-radius: 100px;
  font-size: 11px;
  font-weight: 600;
  color: white;
  text-shadow: 0 1px 2px rgba(0,0,0,0.2);
}

.action-buttons {
  display: flex;
  gap: 8px;
}

.action-btn {
  border: none;
  padding: 6px 12px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
  transition: all 0.2s;
}

.sync-btn {
  background: rgba(59, 130, 246, 0.15);
  color: #60a5fa;
  border: 1px solid rgba(59, 130, 246, 0.3);
}

.sync-btn:hover:not(:disabled) {
  background: rgba(59, 130, 246, 0.25);
}

.view-btn {
  background: rgba(16, 185, 129, 0.15);
  color: #34d399;
  border: 1px solid rgba(16, 185, 129, 0.3);
}

.view-btn:hover:not(:disabled) {
  background: rgba(16, 185, 129, 0.25);
}

.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* Modal Styles */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal-content {
  width: 90%;
  max-width: 800px;
  max-height: 80vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
}

.modal-header {
  padding: 20px 24px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.modal-header h3 {
  margin: 0;
  font-size: 20px;
  color: #f1f5f9;
}

.close-btn {
  background: none;
  border: none;
  color: #94a3b8;
  font-size: 20px;
  cursor: pointer;
}
.close-btn:hover {
  color: #fff;
}

.modal-body {
  padding: 24px;
  overflow-y: auto;
}

.price-val {
  font-family: 'JetBrains Mono', monospace;
  color: #fbbf24;
}

.status-badge {
  padding: 4px 10px;
  border-radius: 100px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
}

.status-badge.active {
  background: rgba(16, 185, 129, 0.15);
  color: #34d399;
}

.status-badge.inactive {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
}

.icon {
  font-size: 14px;
}
</style>
