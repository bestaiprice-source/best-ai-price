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
          <h2>💡 Cheaper Alternatives</h2>
          <p class="subtitle">Discover cheaper upstream provider routes from BestAIPrice database evaluated by Model Family average price.</p>
        </div>
        <div class="actions">
          <button class="primary-btn" @click="handleCheckOpportunities" :disabled="checkingOpportunities">
            <span class="icon">🔍</span> {{ checkingOpportunities ? 'Checking BestAIPrice...' : 'Check BestAIPrice Now' }}
          </button>
        </div>
      </div>

      <!-- Cheaper Provider Opportunities Section -->
      <div class="opportunities-section">
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
            <!-- Family Header Bar -->
            <div class="family-group-header px-4 py-3">
              <div class="family-title-wrap">
                <span class="family-pill" :class="'family-' + group.familyId">
                  {{ group.familyName }}
                </span>
                <span class="family-models-count">
                  {{ group.activeProvider.items.length }} models compared
                </span>
              </div>

              <!-- Candidate Selector Pills if multiple exist -->
              <div v-if="group.candidateProviders.length > 1" class="candidate-selector-bar">
                <span class="selector-label text-muted">Candidates:</span>
                <button
                  v-for="(cand, idx) in group.candidateProviders"
                  :key="cand.domain"
                  class="candidate-pill-btn"
                  :class="{ active: group.activeProvider.domain === cand.domain }"
                  @click="selectedCandidateForFamily[group.familyId] = cand.domain"
                >
                  <span v-if="idx === 0" class="best-star" title="Best Savings">★</span>
                  <span class="cand-name">{{ cand.name }}</span>
                  <span class="cand-savings">-{{ cand.avgSavings.toFixed(1) }}%</span>
                </button>
              </div>

              <!-- Average Price Across All Models of Family -->
              <div class="family-savings-summary">
                <div class="price-stat">
                  <span class="stat-label">Current Avg:</span>
                  <span class="stat-val">${{ group.activeProvider.currentAvgPrice.toFixed(2) }}</span>
                </div>
                <span class="stat-arrow">→</span>
                <div class="price-stat">
                  <span class="stat-label">Candidate Avg:</span>
                  <span class="stat-val text-success">${{ group.activeProvider.candidateAvgPrice.toFixed(2) }}</span>
                </div>
                <span class="savings-badge ml-2">-{{ group.activeProvider.avgSavings.toFixed(1) }}%</span>
              </div>
            </div>

            <!-- Candidate Provider Info Subbar -->
            <div class="candidate-info-subbar px-4 py-2">
              <div class="candidate-meta">
                <span class="text-muted">Candidate:</span> <strong class="text-accent">{{ group.activeProvider.name }}</strong> <span class="sub-text text-muted">({{ group.activeProvider.domain }})</span>
                <span class="mx-2 text-muted">|</span>
                <span class="text-muted">Current:</span> {{ group.activeProvider.currentPrimaryProviders.join(', ') || 'Active' }}
              </div>
              <a v-if="group.activeProvider.cardUrl" :href="group.activeProvider.cardUrl" target="_blank" class="card-link">
                <span>BestAIPrice Card</span> <span class="ext-icon">↗</span>
              </a>
            </div>

            <!-- Table of Models for this Family -->
            <div class="table-container">
              <table class="vendors-table">
                <thead>
                  <tr>
                    <th>Model</th>
                    <th title="Combined benchmark price: 1M Input + 1M Output tokens">Current Route & Price (1M In+Out)</th>
                    <th title="Combined benchmark price: 1M Input + 1M Output tokens">Candidate Price (1M In+Out)</th>
                    <th>Savings</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="opp in group.activeProvider.items" :key="opp.id">
                    <td>
                      <div class="model-cell">
                        <div class="fw-bold">{{ opp.model_name }}</div>
                        <div v-if="opp.canonical_model_id && opp.canonical_model_id !== opp.model_name" class="sub-text text-muted">
                          {{ opp.canonical_model_id }}
                        </div>
                      </div>
                    </td>
                    <td>
                      <div class="route-price-cell">
                        <span class="route-badge primary">Primary:</span>
                        <span class="provider-name">{{ opp.current_primary_provider }}</span>
                        <span class="price-val ml-1">${{ opp.current_primary_price.toFixed(4) }} / 1M</span>
                      </div>
                      <div v-if="opp.current_backup_provider" class="route-price-cell mt-1">
                        <span class="route-badge backup">Backup:</span>
                        <span class="provider-name text-muted">{{ opp.current_backup_provider }}</span>
                        <span class="price-val text-muted ml-1">${{ (opp.current_backup_price || 0).toFixed(4) }} / 1M</span>
                      </div>
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
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import AppLayout from '@/components/layout/AppLayout.vue';
import { 
  getProviderOpportunities,
  checkProviderOpportunities,
  CheaperProviderOpportunity
} from '@/api/admin/vendor';
import { useAppStore } from '@/stores';

const appStore = useAppStore();

// Cheaper Provider Opportunities state
const opportunities = ref<CheaperProviderOpportunity[]>([]);
const loadingOpportunities = ref(false);
const checkingOpportunities = ref(false);
const opportunityStatusFilter = ref('detected');
const selectedFamilyFilter = ref<string>('all');
const selectedCandidateForFamily = ref<Record<string, string>>({});

const MODEL_FAMILIES = [
  { id: 'all', name: 'All Families' },
  { id: 'claude', name: 'Claude' },
  { id: 'gpt', name: 'GPT' },
  { id: 'gemini', name: 'Gemini' },
  { id: 'kimi', name: 'Kimi' },
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
    all: 0,
    claude: 0,
    gpt: 0,
    gemini: 0,
    kimi: 0,
  };
  for (const opp of opportunities.value) {
    const fam = getOpportunityFamily(opp);
    if (fam !== 'other' && counts[fam] !== undefined) {
      counts[fam]++;
      counts.all++;
    }
  }
  return MODEL_FAMILIES.map(f => ({
    id: f.id,
    name: f.name,
    count: counts[f.id] || 0,
  }));
});

interface CandidateProviderGroup {
  name: string;
  domain: string;
  cardUrl: string;
  currentPrimaryProviders: string[];
  items: CheaperProviderOpportunity[];
  currentAvgPrice: number;
  candidateAvgPrice: number;
  avgSavings: number;
}

interface FamilyOpportunityGroup {
  familyId: string;
  familyName: string;
  candidateProviders: CandidateProviderGroup[];
  activeProvider: CandidateProviderGroup;
}

const displayedFamilyGroups = computed<FamilyOpportunityGroup[]>(() => {
  const familyOrder = ['claude', 'gpt', 'gemini', 'kimi'];
  const grouped: Record<string, Record<string, CheaperProviderOpportunity[]>> = {};

  for (const opp of opportunities.value) {
    const fam = getOpportunityFamily(opp);
    if (fam === 'other') continue;
    if (!grouped[fam]) grouped[fam] = {};
    const domain = (opp.candidate_domain || '').toLowerCase().trim();
    if (!grouped[fam][domain]) grouped[fam][domain] = [];
    grouped[fam][domain].push(opp);
  }

  const result: FamilyOpportunityGroup[] = [];
  const familiesToInclude = selectedFamilyFilter.value === 'all'
    ? familyOrder
    : [selectedFamilyFilter.value];

  for (const famId of familiesToInclude) {
    const domainMap = grouped[famId] || {};
    const candidateProviders: CandidateProviderGroup[] = [];

    for (const [domain, items] of Object.entries(domainMap)) {
      if (items.length === 0) continue;
      const currentSum = items.reduce((acc, curr) => acc + (curr.current_primary_price || 0), 0);
      const candidateSum = items.reduce((acc, curr) => acc + (curr.candidate_price || 0), 0);
      const currentAvg = currentSum / items.length;
      const candidateAvg = candidateSum / items.length;
      const savings = currentAvg > 0 ? ((currentAvg - candidateAvg) / currentAvg) * 100 : 0;

      const first = items[0];
      const cardUrl = first.provider_card_url || `https://bestaiprice.com/provider/${domain}`;
      const primSet = new Set<string>();
      items.forEach(it => { if (it.current_primary_provider) primSet.add(it.current_primary_provider); });

      candidateProviders.push({
        name: first.candidate_provider || domain,
        domain,
        cardUrl,
        currentPrimaryProviders: Array.from(primSet),
        items,
        currentAvgPrice: currentAvg,
        candidateAvgPrice: candidateAvg,
        avgSavings: savings,
      });
    }

    if (candidateProviders.length > 0) {
      // Sort candidate providers by average savings descending (best option first)
      candidateProviders.sort((a, b) => b.avgSavings - a.avgSavings);

      // Determine active provider domain
      const selectedDomain = selectedCandidateForFamily.value[famId];
      const activeProvider = candidateProviders.find(p => p.domain === selectedDomain) || candidateProviders[0];

      result.push({
        familyId: famId,
        familyName: formatFamilyTitle(famId),
        candidateProviders,
        activeProvider,
      });
    }
  }

  return result;
});

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
    const count = res.discovered_count !== undefined ? res.discovered_count : (res.opportunities?.length || 0);
    appStore.showSuccess(`${res.message || 'Check completed'}: ${count} cheaper model opportunities found across best family providers`);
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

onMounted(() => {
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

.mt-1 {
  margin-top: 4px;
}

.ml-1 {
  margin-left: 4px;
}

.ml-2 {
  margin-left: 8px;
}

.mx-2 {
  margin-left: 8px;
  margin-right: 8px;
}

.px-4 {
  padding-left: 16px;
  padding-right: 16px;
}

.py-2 {
  padding-top: 8px;
  padding-bottom: 8px;
}

.py-3 {
  padding-top: 12px;
  padding-bottom: 12px;
}

.py-5 {
  padding-top: 40px;
  padding-bottom: 40px;
}

.mb-4 {
  margin-bottom: 16px;
}

.text-center {
  text-align: center;
}

.text-muted {
  color: #64748b;
}

.text-accent {
  color: #818cf8;
}

.text-success {
  color: #10b981;
}

.fw-bold {
  font-weight: 600;
  color: #f1f5f9;
}

.icon {
  font-size: 14px;
}

.vendors-table {
  width: 100%;
  border-collapse: collapse;
}

.vendors-table th {
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

.vendors-table td {
  padding: 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  font-size: 14px;
}

.vendors-table tr:hover {
  background: rgba(255, 255, 255, 0.02);
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

.status-badge.promoted {
  background: rgba(99, 102, 241, 0.15);
  color: #a5b4fc;
}

.status-badge.inactive {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
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
  flex-wrap: wrap;
  gap: 12px;
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

.candidate-selector-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.selector-label {
  font-size: 12px;
  font-weight: 600;
}

.candidate-pill-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.12);
  color: #cbd5e1;
  padding: 4px 10px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s ease;
}

.candidate-pill-btn:hover {
  background: rgba(255, 255, 255, 0.1);
  color: #ffffff;
}

.candidate-pill-btn.active {
  background: rgba(99, 102, 241, 0.25);
  border-color: rgba(99, 102, 241, 0.6);
  color: #c7d2fe;
  box-shadow: 0 0 10px rgba(99, 102, 241, 0.3);
}

.best-star {
  color: #fbbf24;
  font-size: 12px;
}

.cand-savings {
  color: #34d399;
  font-size: 11px;
  font-family: 'JetBrains Mono', monospace;
}

.candidate-info-subbar {
  background: rgba(15, 23, 42, 0.4);
  border-bottom: 1px solid rgba(255, 255, 255, 0.04);
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  font-size: 13px;
}
</style>
