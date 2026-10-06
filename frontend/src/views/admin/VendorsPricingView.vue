<template>
  <AppLayout>
    <div class="vendors-container">
      <div class="header-section">
        <div class="header-content">
          <div class="mb-2">
            <router-link to="/admin/providers" class="inline-flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-primary-500 hover:text-primary-600 transition-colors">
              <span>← Back to Providers</span>
            </router-link>
          </div>
          <h2>AI Providers, Pricing & Margins</h2>
          <p class="subtitle">Manage upstream provider prices, view historical trends, and audit retail gross margins.</p>
        </div>
        <div class="actions">
          <div class="tab-pill-group">
            <button 
              class="tab-pill-btn" 
              :class="{ active: activeTab === 'vendors' }" 
              @click="activeTab = 'vendors'">
              <span class="icon">🏷️</span> Providers & Pricing
            </button>
            <button 
              class="tab-pill-btn" 
              :class="{ active: activeTab === 'analytics' }" 
              @click="switchTab('analytics')">
              <span class="icon">📊</span> Gross Margin Analytics
            </button>
            <button 
              class="tab-pill-btn" 
              :class="{ active: activeTab === 'opportunities' }" 
              @click="switchTab('opportunities')">
              <span class="icon">💡</span> Cheaper Alternatives
              <span v-if="opportunities.length > 0" class="mini-counter">{{ opportunities.length }}</span>
            </button>
          </div>
          <button v-if="activeTab === 'vendors'" class="primary-btn" @click="handleSyncAll" :disabled="isSyncingAll">
            <span class="icon">🔄</span> {{ isSyncingAll ? 'Syncing...' : 'Sync All Prices' }}
          </button>
          <button v-if="activeTab === 'analytics'" class="primary-btn" @click="loadAnalytics" :disabled="loadingAnalytics">
            <span class="icon">🔄</span> {{ loadingAnalytics ? 'Refreshing...' : 'Refresh Stats' }}
          </button>
          <button v-if="activeTab === 'opportunities'" class="primary-btn" @click="handleCheckOpportunities" :disabled="checkingOpportunities">
            <span class="icon">🔍</span> {{ checkingOpportunities ? 'Checking BestAIPrice...' : 'Check BestAIPrice Now' }}
          </button>
        </div>
      </div>

      <!-- Tab 1: Vendors & Models -->
      <div v-if="activeTab === 'vendors'" class="table-container glass-panel">
        <table class="vendors-table">
          <thead>
            <tr>
              <th>Provider</th>
              <th>URL</th>
              <th>Models Count</th>
              <th>Last Updated</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="vendor in displayVendors" :key="vendor.url">
              <td class="fw-bold">{{ vendor.name }}</td>
              <td><a :href="vendor.url" target="_blank">{{ vendor.url }}</a></td>
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

      <!-- Tab 2: Gross Margin Analytics -->
      <div v-if="activeTab === 'analytics'" class="analytics-section">
        <!-- KPI Cards -->
        <div class="kpi-grid">
          <div class="kpi-card glass-panel">
            <div class="kpi-label">Gross Revenue</div>
            <div class="kpi-value text-accent">${{ (marginAnalytics?.total_revenue || 0).toFixed(4) }}</div>
            <div class="kpi-sub">Total billed to clients</div>
          </div>
          <div class="kpi-card glass-panel">
            <div class="kpi-label">Upstream Cost</div>
            <div class="kpi-value text-warning">${{ (marginAnalytics?.total_upstream_cost || 0).toFixed(4) }}</div>
            <div class="kpi-sub">Total paid to suppliers</div>
          </div>
          <div class="kpi-card glass-panel">
            <div class="kpi-label">Net Gross Profit</div>
            <div class="kpi-value" :class="(marginAnalytics?.total_gross_profit || 0) >= 0 ? 'text-success' : 'text-danger'">
              ${{ (marginAnalytics?.total_gross_profit || 0).toFixed(4) }}
            </div>
            <div class="kpi-sub">Revenue - Upstream Cost</div>
          </div>
          <div class="kpi-card glass-panel">
            <div class="kpi-label">Gross Margin %</div>
            <div class="kpi-value" :class="(marginAnalytics?.overall_gross_margin_percent || 0) >= 10 ? 'text-success' : 'text-warning'">
              {{ (marginAnalytics?.overall_gross_margin_percent || 0).toFixed(2) }}%
            </div>
            <div class="kpi-sub">Target min: 10.00%</div>
          </div>
        </div>

        <!-- Breakdown by Model -->
        <div class="table-container glass-panel mt-4">
          <div class="section-title-bar">
            <h3>Margin Breakdown by Model</h3>
            <span class="badge">{{ marginAnalytics?.by_model?.length || 0 }} Models</span>
          </div>
          <table class="vendors-table">
            <thead>
              <tr>
                <th>Model</th>
                <th>Requests</th>
                <th>Tokens</th>
                <th>Revenue</th>
                <th>Upstream Cost</th>
                <th>Gross Profit</th>
                <th>Margin %</th>
                <th>Price History</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!marginAnalytics?.by_model || marginAnalytics.by_model.length === 0">
                <td colspan="8" class="text-center text-muted py-4">No model usage recorded yet.</td>
              </tr>
              <tr v-for="item in marginAnalytics?.by_model" :key="item.name">
                <td class="fw-bold">{{ item.name }}</td>
                <td>{{ item.requests.toLocaleString() }}</td>
                <td>{{ item.total_tokens.toLocaleString() }}</td>
                <td class="price-val">${{ item.revenue.toFixed(4) }}</td>
                <td class="price-val text-warning">${{ item.upstream_cost.toFixed(4) }}</td>
                <td class="price-val" :class="item.gross_profit >= 0 ? 'text-success' : 'text-danger'">
                  ${{ item.gross_profit.toFixed(4) }}
                </td>
                <td>
                  <span class="margin-badge" :class="getMarginClass(item.gross_margin_percent)">
                    {{ item.gross_margin_percent.toFixed(2) }}%
                  </span>
                </td>
                <td>
                  <button class="action-btn history-btn" @click="openPriceHistoryModal('', item.name)">
                    <span class="icon">📈</span> History
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Breakdown by Provider -->
        <div class="table-container glass-panel mt-4">
          <div class="section-title-bar">
            <h3>Margin Breakdown by Provider</h3>
            <span class="badge">{{ marginAnalytics?.by_provider?.length || 0 }} Providers</span>
          </div>
          <table class="vendors-table">
            <thead>
              <tr>
                <th>Provider</th>
                <th>Requests</th>
                <th>Tokens</th>
                <th>Revenue</th>
                <th>Upstream Cost</th>
                <th>Gross Profit</th>
                <th>Margin %</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!marginAnalytics?.by_provider || marginAnalytics.by_provider.length === 0">
                <td colspan="7" class="text-center text-muted py-4">No provider usage recorded yet.</td>
              </tr>
              <tr v-for="item in marginAnalytics?.by_provider" :key="item.name">
                <td class="fw-bold">{{ item.name }}</td>
                <td>{{ item.requests.toLocaleString() }}</td>
                <td>{{ item.total_tokens.toLocaleString() }}</td>
                <td class="price-val">${{ item.revenue.toFixed(4) }}</td>
                <td class="price-val text-warning">${{ item.upstream_cost.toFixed(4) }}</td>
                <td class="price-val" :class="item.gross_profit >= 0 ? 'text-success' : 'text-danger'">
                  ${{ item.gross_profit.toFixed(4) }}
                </td>
                <td>
                  <span class="margin-badge" :class="getMarginClass(item.gross_margin_percent)">
                    {{ item.gross_margin_percent.toFixed(2) }}%
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Tab 3: Cheaper Provider Opportunities -->
      <div v-if="activeTab === 'opportunities'" class="opportunities-section">
        <div class="opportunities-header glass-panel">
          <div class="opp-info">
            <h3>Discovered Cheaper Providers (BestAIPrice Candidates)</h3>
            <p>Regularly compares active Primary & Backup routes against BestAIPrice database grouped by Model Family. Cheaper candidate providers are identified relative to your currently active providers.</p>
          </div>
          <div class="filter-controls">
            <div class="filter-group">
              <label>Status Filter:</label>
              <select v-model="opportunityStatusFilter" @change="loadOpportunities" class="filter-select">
                <option value="">All Statuses</option>
                <option value="detected">Detected (Pending)</option>
                <option value="promoted_primary">Promoted to Primary</option>
                <option value="promoted_backup">Promoted to Backup</option>
                <option value="dismissed">Dismissed</option>
              </select>
            </div>
          </div>
        </div>

        <!-- Family Filter Tabs -->
        <div class="family-filter-bar mt-4">
          <div class="family-tabs">
            <button
              v-for="fam in familyFilterTabs"
              :key="fam.id"
              class="family-tab-btn"
              :class="{ active: selectedFamilyFilter === fam.id }"
              @click="selectedFamilyFilter = fam.id"
            >
              <span class="family-name">{{ fam.name }}</span>
              <span class="family-count-badge" :class="'badge-' + fam.id">{{ fam.count }}</span>
            </button>
          </div>
        </div>

        <!-- Opportunities Content -->
        <div v-if="loadingOpportunities" class="table-container glass-panel mt-4 text-center text-muted py-5">
          Loading opportunities...
        </div>
        <div v-else-if="opportunities.length === 0" class="table-container glass-panel mt-4 text-center text-muted py-5">
          No cheaper opportunities detected for current models. Click "Check BestAIPrice Now" to scan.
        </div>
        <div v-else-if="displayedFamilyGroups.length === 0" class="table-container glass-panel mt-4 text-center text-muted py-5">
          No cheaper opportunities detected for the selected family.
        </div>
        <div v-else class="family-groups-container mt-4">
          <div 
            v-for="group in displayedFamilyGroups" 
            :key="group.familyId" 
            class="family-group-panel glass-panel mb-4"
          >
            <div class="family-group-header">
              <div class="family-title-wrap">
                <span class="family-pill" :class="'family-' + group.familyId">
                  {{ group.familyName }}
                </span>
                <span class="family-models-count">{{ group.items.length }} alternatives</span>
              </div>
              <div class="family-savings-summary">
                <div class="price-stat">
                  <span class="stat-label">Current Avg:</span>
                  <span class="stat-val">${{ group.currentAvgPrice.toFixed(2) }}</span>
                </div>
                <span class="stat-arrow">→</span>
                <div class="price-stat">
                  <span class="stat-label">Candidate Avg:</span>
                  <span class="stat-val text-success">${{ group.candidateAvgPrice.toFixed(2) }}</span>
                </div>
                <span class="savings-badge ml-2">-{{ group.avgSavings.toFixed(1) }}%</span>
              </div>
            </div>

            <div class="table-responsive">
              <table class="vendors-table">
                <thead>
                  <tr>
                    <th>Model</th>
                    <th>Current Route & Price</th>
                    <th>BestAIPrice Candidate</th>
                    <th>Candidate Price</th>
                    <th>Savings</th>
                    <th>BestAIPrice Card</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="opp in group.items" :key="opp.id">
                    <td>
                      <div class="model-cell">
                        <div class="fw-bold">{{ opp.model_name }}</div>
                        <div class="sub-text text-muted">{{ opp.canonical_model_id }}</div>
                      </div>
                    </td>
                    <td>
                      <div><span class="route-badge primary">Primary:</span> {{ opp.current_primary_provider }}</div>
                      <div class="price-val">${{ opp.current_primary_price.toFixed(4) }} / 1M</div>
                      <div v-if="opp.current_backup_provider" class="mt-1">
                        <span class="route-badge backup">Backup:</span> {{ opp.current_backup_provider }}
                        <span class="price-val text-muted">${{ (opp.current_backup_price || 0).toFixed(4) }} / 1M</span>
                      </div>
                    </td>
                    <td>
                      <div class="fw-bold text-accent">{{ opp.candidate_provider }}</div>
                      <div class="sub-text text-muted">{{ opp.candidate_domain }}</div>
                    </td>
                    <td>
                      <div class="price-val text-success">${{ opp.candidate_price.toFixed(4) }} / 1M</div>
                      <div v-if="opp.candidate_input_price != null" class="sub-text text-muted">
                        In: ${{ opp.candidate_input_price.toFixed(3) }} / Out: ${{ (opp.candidate_output_price || 0).toFixed(3) }}
                      </div>
                    </td>
                    <td>
                      <span class="savings-badge">
                        -{{ opp.diff_percent.toFixed(1) }}%
                      </span>
                    </td>
                    <td>
                      <a v-if="opp.provider_card_url" :href="opp.provider_card_url" target="_blank" class="card-link">
                        <span>Card</span> <span class="ext-icon">↗</span>
                      </a>
                      <span v-else class="text-muted">—</span>
                    </td>
                    <td>
                      <span class="status-badge" :class="getStatusBadgeClass(opp.status)">
                        {{ formatOpportunityStatus(opp.status) }}
                      </span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
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
                <th>History</th>
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
                <td>
                  <button class="action-btn history-btn" @click="openPriceHistoryModal(selectedVendor?.id, model.model_name)">
                    <span class="icon">📈</span>
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Price History Modal -->
    <div v-if="historyModalVisible" class="modal-overlay" @click.self="closePriceHistoryModal">
      <div class="modal-content glass-panel history-modal">
        <div class="modal-header">
          <h3>Price History: {{ historyModelName || 'All Models' }}</h3>
          <button class="close-btn" @click="closePriceHistoryModal">✕</button>
        </div>
        <div class="modal-body">
          <div v-if="loadingHistory" class="text-center py-4 text-muted">
            Loading historical price snapshots...
          </div>
          <div v-else-if="priceHistory.length === 0" class="text-center py-4 text-muted">
            No price history snapshots recorded yet for this model/provider.
          </div>
          <table v-else class="models-table">
            <thead>
              <tr>
                <th>Recorded At</th>
                <th>Model</th>
                <th>Input Price / 1M</th>
                <th>Output Price / 1M</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in priceHistory" :key="item.id">
                <td>{{ formatDate(item.recorded_at) }}</td>
                <td class="fw-bold">{{ item.model_name }}</td>
                <td class="price-val">${{ item.input_price.toFixed(6) }}</td>
                <td class="price-val">${{ item.output_price.toFixed(6) }}</td>
                <td>
                  <span class="status-badge" :class="item.is_available ? 'active' : 'inactive'">
                    {{ item.is_available ? 'Available' : 'Unavailable' }}
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
import { useRoute } from 'vue-router';
import AppLayout from '@/components/layout/AppLayout.vue';
import { 
  getVendors, 
  scrapeVendorByUrl, 
  getVendorPriceHistory, 
  getGrossMarginAnalytics,
  getProviderOpportunities,
  checkProviderOpportunities,
  Vendor, 
  VendorPriceHistoryItem, 
  GrossMarginAnalytics,
  CheaperProviderOpportunity
} from '@/api/admin/vendor';
import { useAppStore } from '@/stores'

const appStore = useAppStore();
const activeTab = ref<'vendors' | 'analytics' | 'opportunities'>('vendors');
const dbVendors = ref<Vendor[]>([]);
const loading = ref(true);
const syncingId = ref<string | null>(null);
const isSyncingAll = ref(false);
const selectedVendor = ref<Vendor | null>(null);

// Price History Modal state
const historyModalVisible = ref(false);
const loadingHistory = ref(false);
const priceHistory = ref<VendorPriceHistoryItem[]>([]);
const historyModelName = ref('');

// Gross Margin Analytics state
const loadingAnalytics = ref(false);
const marginAnalytics = ref<GrossMarginAnalytics | null>(null);

// Cheaper Provider Opportunities state
const opportunities = ref<CheaperProviderOpportunity[]>([]);
const loadingOpportunities = ref(false);
const checkingOpportunities = ref(false);
const opportunityStatusFilter = ref('detected');
const selectedFamilyFilter = ref<string>('all');

const MODEL_FAMILIES = [
  { id: 'all', name: 'All Families' },
  { id: 'claude', name: 'Claude' },
  { id: 'gpt', name: 'GPT' },
  { id: 'gemini', name: 'Gemini' },
  { id: 'kimi', name: 'Kimi' },
  { id: 'other', name: 'Other' },
];

const getOpportunityFamily = (opp: CheaperProviderOpportunity): string => {
  if (opp.model_family) {
    return opp.model_family.toLowerCase();
  }
  const m = (opp.canonical_model_id || opp.model_name || '').toLowerCase();
  if (m.includes('claude') || m.includes('anthropic')) return 'claude';
  if (m.includes('gpt') || m.includes('openai') || m.startsWith('o1') || m.startsWith('o3')) return 'gpt';
  if (m.includes('gemini') || m.includes('google')) return 'gemini';
  if (m.includes('kimi') || m.includes('moonshot')) return 'kimi';
  return 'other';
};

const formatFamilyTitle = (familyId: string): string => {
  switch (familyId.toLowerCase()) {
    case 'claude': return 'Claude (Anthropic)';
    case 'gpt': return 'GPT (OpenAI)';
    case 'gemini': return 'Gemini (Google)';
    case 'kimi': return 'Kimi (Moonshot)';
    default: return 'Other Models';
  }
};

const familyFilterTabs = computed(() => {
  const counts: Record<string, number> = {
    all: opportunities.value.length,
    claude: 0,
    gpt: 0,
    gemini: 0,
    kimi: 0,
    other: 0,
  };
  for (const opp of opportunities.value) {
    const fam = getOpportunityFamily(opp);
    if (counts[fam] !== undefined) {
      counts[fam]++;
    } else {
      counts.other++;
    }
  }
  return MODEL_FAMILIES.map(f => ({
    id: f.id,
    name: f.name,
    count: counts[f.id] || 0,
  }));
});

interface FamilyOpportunityGroup {
  familyId: string;
  familyName: string;
  items: CheaperProviderOpportunity[];
  currentAvgPrice: number;
  candidateAvgPrice: number;
  avgSavings: number;
}

const displayedFamilyGroups = computed<FamilyOpportunityGroup[]>(() => {
  const familyOrder = ['claude', 'gpt', 'gemini', 'kimi', 'other'];
  const grouped: Record<string, CheaperProviderOpportunity[]> = {};

  for (const opp of opportunities.value) {
    const fam = getOpportunityFamily(opp);
    if (!grouped[fam]) grouped[fam] = [];
    grouped[fam].push(opp);
  }

  const result: FamilyOpportunityGroup[] = [];
  const familiesToInclude = selectedFamilyFilter.value === 'all'
    ? familyOrder
    : [selectedFamilyFilter.value];

  for (const famId of familiesToInclude) {
    const items = grouped[famId] || [];
    if (items.length > 0) {
      const currentSum = items.reduce((acc, curr) => acc + (curr.current_primary_price || 0), 0);
      const candidateSum = items.reduce((acc, curr) => acc + (curr.candidate_price || 0), 0);
      const currentAvg = currentSum / items.length;
      const candidateAvg = candidateSum / items.length;
      const savings = currentAvg > 0 ? ((currentAvg - candidateAvg) / currentAvg) * 100 : 0;

      result.push({
        familyId: famId,
        familyName: formatFamilyTitle(famId),
        items,
        currentAvgPrice: currentAvg,
        candidateAvgPrice: candidateAvg,
        avgSavings: savings,
      });
    }
  }

  return result;
});


const HARDCODED_VENDORS = [
  { name: 'OpenLux (Yunwu)', url: 'https://api.openlux.ai' },
  { name: 'Portdan AI', url: 'https://portdan.com' },
  { name: 'Top-API', url: 'https://api-top.com' },
  { name: 'Yuanqiaoyun', url: 'https://yuanqiaoyun.com' },
  { name: 'X-LLM', url: 'https://x-llm.net' }
];

const displayVendors = computed(() => {
  return HARDCODED_VENDORS.map(hc => {
    const dbMatch = dbVendors.value.find(dbv => dbv.url === hc.url);
    if (dbMatch) {
      return dbMatch;
    }
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
    dbVendors.value = Array.isArray(res) ? res : ((res as any)?.data || []);
  } catch (error: any) {
    console.warn('Failed to load vendors:', error);
  } finally {
    loading.value = false;
  }
};

const switchTab = async (tab: 'vendors' | 'analytics' | 'opportunities') => {
  activeTab.value = tab;
  if (tab === 'analytics' && !marginAnalytics.value) {
    await loadAnalytics();
  } else if (tab === 'opportunities') {
    await loadOpportunities();
  }
};

const loadAnalytics = async () => {
  try {
    loadingAnalytics.value = true;
    const res = await getGrossMarginAnalytics();
    marginAnalytics.value = res;
  } catch (error: any) {
    appStore.showError(error.message || 'Failed to load gross margin analytics');
  } finally {
    loadingAnalytics.value = false;
  }
};

const loadOpportunities = async () => {
  try {
    loadingOpportunities.value = true;
    const res = await getProviderOpportunities(opportunityStatusFilter.value);
    opportunities.value = Array.isArray(res) ? res : ((res as any)?.data || []);
  } catch (error: any) {
    console.warn('Failed to load opportunities:', error);
  } finally {
    loadingOpportunities.value = false;
  }
};

const handleCheckOpportunities = async () => {
  try {
    checkingOpportunities.value = true;
    const res = await checkProviderOpportunities();
    appStore.showSuccess(`${res.message}: ${res.discovered_count} opportunities identified`);
    await loadOpportunities();
  } catch (error: any) {
    appStore.showError(error.message || 'Check failed');
  } finally {
    checkingOpportunities.value = false;
  }
};


const getStatusBadgeClass = (status: string) => {
  switch (status) {
    case 'detected': return 'active';
    case 'promoted_primary':
    case 'promoted_backup': return 'promoted';
    case 'dismissed': return 'inactive';
    default: return '';
  }
};

const formatOpportunityStatus = (status: string) => {
  switch (status) {
    case 'detected': return 'Detected';
    case 'promoted_primary': return 'Primary';
    case 'promoted_backup': return 'Backup';
    case 'dismissed': return 'Dismissed';
    default: return status;
  }
};

const handleScrape = async (vendor: Vendor) => {
  try {
    syncingId.value = vendor.url;
    const res = await scrapeVendorByUrl(vendor.name, vendor.url);
    appStore.showSuccess(res.message + ` (${res.models_count} models synced)`);
    await loadVendors();
  } catch (error: any) {
    appStore.showError(error.message || `Failed to sync ${vendor.name}`);
  } finally {
    syncingId.value = null;
  }
};

const handleSyncAll = async () => {
  try {
    isSyncingAll.value = true;
    let syncedCount = 0;
    const failedVendors: string[] = [];
    for (const vendor of displayVendors.value) {
      try {
        await scrapeVendorByUrl(vendor.name, vendor.url);
        syncedCount++;
      } catch (err: any) {
        console.warn(`Failed to sync vendor ${vendor.name}:`, err);
        failedVendors.push(vendor.name);
      }
    }
    if (failedVendors.length === 0) {
      appStore.showSuccess(`All ${syncedCount} vendors synced successfully`);
    } else if (syncedCount > 0) {
      appStore.showWarning(`Synced ${syncedCount} vendors (${failedVendors.join(', ')} failed)`);
    } else {
      appStore.showError(`Failed to sync vendors: ${failedVendors.join(', ')}`);
    }
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

const openPriceHistoryModal = async (vendorId?: string, modelName?: string) => {
  try {
    historyModalVisible.value = true;
    loadingHistory.value = true;
    historyModelName.value = modelName || '';
    const res = await getVendorPriceHistory(vendorId, modelName);
    priceHistory.value = res || [];
  } catch (error: any) {
    appStore.showError(error.message || 'Failed to load price history');
  } finally {
    loadingHistory.value = false;
  }
};

const closePriceHistoryModal = () => {
  historyModalVisible.value = false;
  priceHistory.value = [];
  historyModelName.value = '';
};

const getMarginClass = (pct: number) => {
  if (pct >= 20) return 'margin-high';
  if (pct >= 10) return 'margin-good';
  if (pct >= 0) return 'margin-low';
  return 'margin-negative';
};

const route = useRoute();

const formatDate = (dateStr: string) => {
  if (!dateStr) return 'Never';
  return new Date(dateStr).toLocaleString();
};

onMounted(() => {
  if (route.query.tab === 'opportunities') {
    activeTab.value = 'opportunities';
  } else if (route.query.tab === 'analytics') {
    activeTab.value = 'analytics';
  }
  loadVendors();
  loadOpportunities();
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
  margin-bottom: 28px;
  flex-wrap: wrap;
  gap: 16px;
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

.actions {
  display: flex;
  align-items: center;
  gap: 16px;
}

.tab-pill-group {
  display: flex;
  background: rgba(15, 23, 42, 0.7);
  padding: 4px;
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.tab-pill-btn {
  background: transparent;
  border: none;
  color: #94a3b8;
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 8px;
  transition: all 0.2s ease;
}

.tab-pill-btn.active {
  background: rgba(99, 102, 241, 0.2);
  color: #a5b4fc;
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.25);
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

.mt-4 {
  margin-top: 24px;
}

.section-title-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.section-title-bar h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: #f1f5f9;
}

.badge {
  background: rgba(99, 102, 241, 0.15);
  color: #a5b4fc;
  font-size: 12px;
  font-weight: 600;
  padding: 4px 10px;
  border-radius: 12px;
}

/* KPI Cards */
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}

.kpi-card {
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.kpi-label {
  font-size: 13px;
  font-weight: 600;
  text-transform: uppercase;
  color: #94a3b8;
  letter-spacing: 0.04em;
}

.kpi-value {
  font-size: 26px;
  font-weight: 700;
  font-family: 'JetBrains Mono', monospace;
}

.kpi-sub {
  font-size: 12px;
  color: #64748b;
}

.text-accent {
  color: #818cf8;
}

.text-warning {
  color: #f59e0b;
}

.text-success {
  color: #10b981;
}

.text-danger {
  color: #ef4444;
}

.text-center {
  text-align: center;
}

.py-4 {
  padding-top: 16px;
  padding-bottom: 16px;
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

.history-btn {
  background: rgba(168, 85, 247, 0.15);
  color: #c084fc;
  border: 1px solid rgba(168, 85, 247, 0.3);
}

.history-btn:hover:not(:disabled) {
  background: rgba(168, 85, 247, 0.25);
}

.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* Margin Badges */
.margin-badge {
  padding: 4px 8px;
  border-radius: 6px;
  font-weight: 600;
  font-size: 12px;
  font-family: 'JetBrains Mono', monospace;
}

.margin-high {
  background: rgba(16, 185, 129, 0.2);
  color: #34d399;
  border: 1px solid rgba(16, 185, 129, 0.4);
}

.margin-good {
  background: rgba(99, 102, 241, 0.2);
  color: #a5b4fc;
  border: 1px solid rgba(99, 102, 241, 0.4);
}

.margin-low {
  background: rgba(245, 158, 11, 0.2);
  color: #fbbf24;
  border: 1px solid rgba(245, 158, 11, 0.4);
}

.margin-negative {
  background: rgba(239, 68, 68, 0.2);
  color: #f87171;
  border: 1px solid rgba(239, 68, 68, 0.4);
}

/* Modal Styles */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.65);
  backdrop-filter: blur(6px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal-content {
  width: 90%;
  max-width: 860px;
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

/* Opportunities Section */
.opportunities-header {
  padding: 20px 24px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
  border-radius: 12px;
}

.opp-info h3 {
  margin: 0 0 6px 0;
  font-size: 18px;
  color: #f1f5f9;
}

.opp-info p {
  margin: 0;
  font-size: 13px;
  color: #94a3b8;
  max-width: 750px;
}

.filter-group {
  display: flex;
  align-items: center;
  gap: 10px;
}

.filter-group label {
  font-size: 13px;
  color: #94a3b8;
}

.filter-select {
  background: rgba(15, 23, 42, 0.8);
  border: 1px solid rgba(255, 255, 255, 0.15);
  color: #f1f5f9;
  padding: 6px 12px;
  border-radius: 8px;
  font-size: 13px;
  outline: none;
}

.mini-counter {
  margin-left: 6px;
  background: #10b981;
  color: #fff;
  border-radius: 100px;
  padding: 2px 7px;
  font-size: 11px;
  font-weight: 700;
}

.savings-badge {
  display: inline-block;
  padding: 4px 10px;
  border-radius: 6px;
  font-weight: 700;
  font-size: 13px;
  font-family: 'JetBrains Mono', monospace;
  background: rgba(16, 185, 129, 0.2);
  color: #34d399;
  border: 1px solid rgba(16, 185, 129, 0.4);
}

.route-badge {
  font-size: 11px;
  font-weight: 600;
  padding: 2px 6px;
  border-radius: 4px;
  margin-right: 4px;
}

.route-badge.primary {
  background: rgba(59, 130, 246, 0.2);
  color: #60a5fa;
}

.route-badge.backup {
  background: rgba(148, 163, 184, 0.2);
  color: #94a3b8;
}

.sub-text {
  font-size: 11px;
}

.card-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #60a5fa;
  text-decoration: none;
  font-size: 13px;
  font-weight: 500;
}

.card-link:hover {
  text-decoration: underline;
  color: #93c5fd;
}

/* Family Filter Tabs */
.family-filter-bar {
  display: flex;
  align-items: center;
  margin-bottom: 8px;
}

.family-tabs {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  background: rgba(15, 23, 42, 0.6);
  padding: 6px;
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.family-tab-btn {
  background: transparent;
  border: none;
  color: #94a3b8;
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 8px;
  transition: all 0.2s ease;
}

.family-tab-btn:hover {
  color: #f1f5f9;
  background: rgba(255, 255, 255, 0.05);
}

.family-tab-btn.active {
  background: rgba(99, 102, 241, 0.25);
  color: #a5b4fc;
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.25);
}

.family-count-badge {
  font-size: 11px;
  padding: 2px 7px;
  border-radius: 100px;
  background: rgba(255, 255, 255, 0.1);
  color: #cbd5e1;
  font-weight: 700;
}

.family-tab-btn.active .family-count-badge {
  background: rgba(99, 102, 241, 0.4);
  color: #ffffff;
}

/* Family Group Header */
.family-group-panel {
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.family-group-header {
  padding: 16px 20px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: rgba(255, 255, 255, 0.02);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.family-title-wrap {
  display: flex;
  align-items: center;
  gap: 12px;
}

.family-pill {
  padding: 4px 12px;
  border-radius: 20px;
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.family-pill.family-claude {
  background: rgba(168, 85, 247, 0.2);
  color: #c084fc;
  border: 1px solid rgba(168, 85, 247, 0.4);
}

.family-pill.family-gpt {
  background: rgba(16, 185, 129, 0.2);
  color: #34d399;
  border: 1px solid rgba(16, 185, 129, 0.4);
}

.family-pill.family-gemini {
  background: rgba(59, 130, 246, 0.2);
  color: #60a5fa;
  border: 1px solid rgba(59, 130, 246, 0.4);
}

.family-pill.family-kimi {
  background: rgba(6, 182, 212, 0.2);
  color: #22d3ee;
  border: 1px solid rgba(6, 182, 212, 0.4);
}

.family-pill.family-other {
  background: rgba(148, 163, 184, 0.2);
  color: #94a3b8;
  border: 1px solid rgba(148, 163, 184, 0.4);
}

.family-models-count {
  font-size: 13px;
  color: #94a3b8;
}

.family-savings-summary {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.price-stat {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
}

.stat-label {
  font-size: 11px;
  color: #94a3b8;
  text-transform: uppercase;
  font-weight: 600;
}

.stat-val {
  font-family: 'JetBrains Mono', monospace;
  font-weight: 700;
}

.stat-arrow {
  color: #64748b;
  font-size: 13px;
}

.ml-2 {
  margin-left: 8px;
}
</style>

