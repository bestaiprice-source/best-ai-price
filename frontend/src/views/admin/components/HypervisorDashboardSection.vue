<template>
  <div class="card p-6 space-y-6">
    <!-- Section Header -->
    <div class="flex flex-wrap items-center justify-between gap-4 border-b border-gray-200 pb-4 dark:border-gray-700">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-indigo-100 p-2.5 dark:bg-indigo-900/40">
          <Icon name="cpu" size="md" class="text-indigo-600 dark:text-indigo-400" :stroke-width="2" />
        </div>
        <div>
          <h3 class="text-lg font-bold text-gray-900 dark:text-white">
            Hypervisor Engine (ASH)
          </h3>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            Real-time session hypervisor, load balancer & pool health controller
          </p>
        </div>
      </div>

      <div class="flex items-center gap-3">
        <!-- Refresh Button -->
        <button
          @click="loadAshData"
          class="btn btn-secondary text-xs flex items-center gap-1.5"
          :disabled="loading"
        >
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
          Refresh Stats
        </button>

        <!-- System State Badge -->
        <div class="flex items-center gap-2">
          <span class="text-xs font-semibold text-gray-500 dark:text-gray-400">Degradation State:</span>
          <span
            class="inline-flex items-center rounded-md px-2.5 py-1 text-xs font-bold uppercase tracking-wider"
            :class="stateBadgeClass"
          >
            {{ healthData?.system_state || 'NORMAL' }}
          </span>
        </div>
      </div>
    </div>

    <!-- Core Metrics Cards -->
    <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <!-- Total Nodes -->
      <div class="rounded-xl border border-gray-200 bg-gray-50/50 p-4 dark:border-gray-700 dark:bg-gray-800/40">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Pool Nodes</span>
          <span class="rounded bg-emerald-100 px-1.5 py-0.5 text-[10px] font-bold text-emerald-700 dark:bg-emerald-900/50 dark:text-emerald-300">
            {{ healthData?.healthy_nodes || 0 }} Healthy
          </span>
        </div>
        <p class="mt-2 text-2xl font-black text-gray-900 dark:text-white">
          {{ healthData?.total_nodes || 0 }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          Cooldown: {{ healthData?.cooldown_nodes || 0 }}
        </p>
      </div>

      <!-- Active Virtual Sessions -->
      <div class="rounded-xl border border-gray-200 bg-gray-50/50 p-4 dark:border-gray-700 dark:bg-gray-800/40">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Active Sessions</span>
          <Icon name="chart" size="sm" class="text-indigo-500" />
        </div>
        <p class="mt-2 text-2xl font-black text-indigo-600 dark:text-indigo-400">
          {{ globalStats?.active_sessions_count || 0 }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          Virtual Rebinding Active
        </p>
      </div>

      <!-- Total Tokens Handled -->
      <div class="rounded-xl border border-gray-200 bg-gray-50/50 p-4 dark:border-gray-700 dark:bg-gray-800/40">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Input / Output Tokens</span>
          <Icon name="database" size="sm" class="text-amber-500" />
        </div>
        <p class="mt-2 text-xl font-bold text-gray-900 dark:text-white">
          {{ formatTokens((globalStats?.total_input_tokens || 0) + (globalStats?.total_output_tokens || 0)) }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          In: {{ formatTokens(globalStats?.total_input_tokens || 0) }} | Out: {{ formatTokens(globalStats?.total_output_tokens || 0) }}
        </p>
      </div>

      <!-- Total Cost Units -->
      <div class="rounded-xl border border-gray-200 bg-gray-50/50 p-4 dark:border-gray-700 dark:bg-gray-800/40">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Usage Cost Units</span>
          <Icon name="cube" size="sm" class="text-emerald-500" />
        </div>
        <p class="mt-2 text-2xl font-black text-emerald-600 dark:text-emerald-400">
          {{ globalStats?.total_cost_units || 0 }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          Pooled Capacity Units
        </p>
      </div>
    </div>

    <!-- Node Metrics Table -->
    <div class="space-y-3">
      <div class="flex items-center justify-between">
        <h4 class="text-sm font-bold text-gray-900 dark:text-white flex items-center gap-2">
          <span>Execution Node Health & Load Balancing</span>
          <span class="rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 dark:bg-gray-800 dark:text-gray-300">
            {{ nodeMetrics.length }} registered
          </span>
        </h4>
      </div>

      <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
        <table class="w-full text-left text-xs text-gray-600 dark:text-gray-300">
          <thead class="bg-gray-100/70 text-xs font-semibold uppercase tracking-wider text-gray-700 dark:bg-gray-800/60 dark:text-gray-300">
            <tr>
              <th class="px-4 py-3">Node / Account</th>
              <th class="px-4 py-3">Platform</th>
              <th class="px-4 py-3">Status</th>
              <th class="px-4 py-3">Success Rate</th>
              <th class="px-4 py-3">Avg Latency</th>
              <th class="px-4 py-3">Recent 429 / Timeout</th>
              <th class="px-4 py-3">Cooldown Until</th>
              <th class="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 dark:divide-gray-700">
            <tr v-if="nodeMetrics.length === 0">
              <td colspan="8" class="py-8 text-center text-gray-500 dark:text-gray-400">
                No active execution nodes found in Redis / DB registry.
              </td>
            </tr>
            <tr
              v-for="node in nodeMetrics"
              :key="node.node_id"
              class="hover:bg-gray-50/80 dark:hover:bg-gray-800/50 transition-colors"
            >
              <!-- Node Name / ID -->
              <td class="px-4 py-3 font-semibold text-gray-900 dark:text-white">
                <div>{{ node.account_name || ('Node #' + node.node_id) }}</div>
                <div class="text-[10px] font-mono text-gray-400">ID: {{ node.node_id }}</div>
              </td>

              <!-- Provider -->
              <td class="px-4 py-3 uppercase text-xs font-mono font-medium">
                {{ node.provider || 'anthropic' }}
              </td>

              <!-- Status Badge -->
              <td class="px-4 py-3">
                <span
                  class="inline-flex items-center rounded-full px-2.5 py-0.5 text-[11px] font-semibold"
                  :class="getNodeStateClass(node.state)"
                >
                  {{ node.state || 'healthy' }}
                </span>
              </td>

              <!-- Success Rate -->
              <td class="px-4 py-3 font-mono">
                <div class="flex items-center gap-2">
                  <div class="w-16 h-1.5 rounded-full bg-gray-200 dark:bg-gray-700 overflow-hidden">
                    <div
                      class="h-full rounded-full transition-all"
                      :class="node.success_rate >= 0.9 ? 'bg-emerald-500' : node.success_rate >= 0.7 ? 'bg-amber-500' : 'bg-rose-500'"
                      :style="{ width: (node.success_rate * 100) + '%' }"
                    ></div>
                  </div>
                  <span>{{ (node.success_rate * 100).toFixed(0) }}%</span>
                </div>
              </td>

              <!-- Avg Latency -->
              <td class="px-4 py-3 font-mono">
                {{ node.avg_latency_ms || 0 }} ms
              </td>

              <!-- Recent Errors -->
              <td class="px-4 py-3 font-mono">
                <span :class="{ 'text-rose-500 font-bold': node.recent_429 > 0 }">
                  429: {{ node.recent_429 || 0 }}
                </span>
                <span class="mx-1 text-gray-400">|</span>
                <span :class="{ 'text-amber-500 font-bold': node.recent_timeouts > 0 }">
                  Timeout: {{ node.recent_timeouts || 0 }}
                </span>
              </td>

              <!-- Cooldown Until -->
              <td class="px-4 py-3 font-mono text-xs">
                <span v-if="node.cooldown_until" class="text-rose-500 font-semibold">
                  {{ formatDate(node.cooldown_until) }}
                </span>
                <span v-else class="text-gray-400">-</span>
              </td>

              <!-- Actions -->
              <td class="px-4 py-3 text-right">
                <div class="flex items-center justify-end gap-1.5">
                  <button
                    @click="handleCooldown(node.node_id)"
                    class="rounded px-2 py-1 text-[11px] font-medium bg-amber-50 text-amber-700 hover:bg-amber-100 dark:bg-amber-900/30 dark:text-amber-300 dark:hover:bg-amber-900/50"
                    title="Put in 10-minute cooldown"
                  >
                    Cooldown
                  </button>
                  <button
                    v-if="node.state === 'unavailable' || node.state === 'cooldown'"
                    @click="handleEnable(node.node_id)"
                    class="rounded px-2 py-1 text-[11px] font-medium bg-emerald-50 text-emerald-700 hover:bg-emerald-100 dark:bg-emerald-900/30 dark:text-emerald-300 dark:hover:bg-emerald-900/50"
                  >
                    Enable
                  </button>
                  <button
                    v-else
                    @click="handleDisable(node.node_id)"
                    class="rounded px-2 py-1 text-[11px] font-medium bg-rose-50 text-rose-700 hover:bg-rose-100 dark:bg-rose-900/30 dark:text-rose-300 dark:hover:bg-rose-900/50"
                  >
                    Disable
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { adminAPI } from '@/api/admin'
import type { AshHealthResponse, AshGlobalStats, AshNodeMetric } from '@/api/admin/ash'
import Icon from '@/components/icons/Icon.vue'

const loading = ref(false)
const healthData = ref<AshHealthResponse | null>(null)
const globalStats = ref<AshGlobalStats | null>(null)
const nodeMetrics = ref<AshNodeMetric[]>([])
let refreshTimer: ReturnType<typeof setInterval> | null = null

const stateBadgeClass = computed(() => {
  const state = healthData.value?.system_state || 'NORMAL'
  switch (state) {
    case 'NORMAL':
      return 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300'
    case 'WARNING':
      return 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300'
    case 'DEGRADED':
      return 'bg-orange-100 text-orange-800 dark:bg-orange-900/40 dark:text-orange-300'
    case 'SURVIVAL':
      return 'bg-rose-100 text-rose-800 dark:bg-rose-900/40 dark:text-rose-300'
    default:
      return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300'
  }
})

const getNodeStateClass = (state: string) => {
  switch (state) {
    case 'healthy':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
    case 'busy':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
    case 'cooldown':
      return 'bg-rose-100 text-rose-700 dark:bg-rose-900/40 dark:text-rose-300'
    case 'limited':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
    case 'unavailable':
      return 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300'
  }
}

const formatTokens = (val: number) => {
  if (!val) return '0'
  if (val >= 1000000) return (val / 1000000).toFixed(2) + 'M'
  if (val >= 1000) return (val / 1000).toFixed(1) + 'K'
  return val.toString()
}

const formatDate = (dateStr?: string) => {
  if (!dateStr) return '-'
  try {
    const d = new Date(dateStr)
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  } catch {
    return dateStr
  }
}

const loadAshData = async () => {
  loading.value = true
  try {
    const [healthRes, statsRes, nodesRes] = await Promise.all([
      adminAPI.ash.getAshHealth().catch(() => null),
      adminAPI.ash.getAshGlobalStats().catch(() => null),
      adminAPI.ash.getAshNodeMetrics().catch(() => [])
    ])
    healthData.value = healthRes
    globalStats.value = statsRes
    nodeMetrics.value = nodesRes || []
  } catch (err) {
    console.error('Failed to load ASH hypervisor stats:', err)
  } finally {
    loading.value = false
  }
}

const handleCooldown = async (nodeID: string) => {
  try {
    await adminAPI.ash.setAshNodeCooldown(nodeID)
    await loadAshData()
  } catch (err) {
    console.error('Failed to set node cooldown:', err)
  }
}

const handleEnable = async (nodeID: string) => {
  try {
    await adminAPI.ash.enableAshNode(nodeID)
    await loadAshData()
  } catch (err) {
    console.error('Failed to enable node:', err)
  }
}

const handleDisable = async (nodeID: string) => {
  try {
    await adminAPI.ash.disableAshNode(nodeID)
    await loadAshData()
  } catch (err) {
    console.error('Failed to disable node:', err)
  }
}

onMounted(() => {
  loadAshData()
  refreshTimer = setInterval(loadAshData, 10000)
})

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
  }
})
</script>
