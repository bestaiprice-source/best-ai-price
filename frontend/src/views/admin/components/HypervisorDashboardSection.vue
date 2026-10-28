<template>
  <div class="card p-6 space-y-6">
    <!-- Section Header -->
    <div class="flex flex-wrap items-center justify-between gap-4 border-b border-gray-200 pb-4 dark:border-gray-700">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-indigo-100 p-2.5 dark:bg-indigo-900/40">
          <Icon name="cpu" size="md" class="text-indigo-600 dark:text-indigo-400" :stroke-width="2" />
        </div>
        <div>
          <div class="flex items-center gap-2">
            <button
              @click="activeEngine = 'standard'"
              class="px-3 py-1.5 text-xs font-bold rounded-lg transition-colors flex items-center gap-1.5"
              :class="activeEngine === 'standard' ? 'bg-indigo-600 text-white shadow-sm' : 'bg-gray-100 text-gray-700 hover:bg-gray-200 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700'"
            >
              <Icon name="server" size="xs" />
              <span>{{ t('ui.hypervisorDashboardSection.standardASHEngine') }}</span>
            </button>
            <button
              @click="activeEngine = 'yunwu'"
              class="px-3 py-1.5 text-xs font-bold rounded-lg transition-colors flex items-center gap-1.5"
              :class="activeEngine === 'yunwu' ? 'bg-purple-600 text-white shadow-sm' : 'bg-gray-100 text-gray-700 hover:bg-gray-200 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700'"
            >
              <Icon name="bolt" size="xs" />
              <span>{{ t('ui.hypervisorDashboardSection.yunwuHypervisorEngine') }}</span>
              <span
                v-if="yunwuTotalAvailable > 0"
                class="rounded-full px-1.5 py-0.5 text-[10px] font-mono font-semibold"
                :class="activeEngine === 'yunwu' ? 'bg-purple-800 text-purple-100' : 'bg-purple-100 text-purple-700 dark:bg-purple-900/50 dark:text-purple-300'"
              >
                ⚡ {{ formatYunwuCredits(yunwuTotalAvailable) }}
              </span>
            </button>
          </div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ activeEngine === 'standard' ? t('ui.hypervisorDashboardSection.standardEngineDescription') : t('ui.hypervisorDashboardSection.yunwuEngineDescription') }}
          </p>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-3">
        <!-- Sync All Yunwu Balances Button -->
        <button
          v-if="activeEngine === 'yunwu'"
          @click="handleSyncAllYunwu"
          class="btn btn-secondary text-xs flex items-center gap-1.5 bg-purple-50 text-purple-700 hover:bg-purple-100 dark:bg-purple-950/40 dark:text-purple-300 dark:hover:bg-purple-900/50 border-purple-200 dark:border-purple-800"
          :disabled="syncingAll || loading"
          :title="t('ui.hypervisorDashboardSection.querycreditgrantsfrom')"
        >
          <Icon name="sync" size="sm" :class="{ 'animate-spin': syncingAll }" />
          <span>{{ syncingAll ? t('ui.hypervisorDashboardSection.syncingBalances') : t('ui.hypervisorDashboardSection.syncAllBalances') }}</span>
        </button>

        <!-- Refresh Button -->
        <button
          @click="loadAshData"
          class="btn btn-secondary text-xs flex items-center gap-1.5"
          :disabled="loading"
        >
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
          <span>{{ t('ui.hypervisorDashboardSection.refreshStats') }}</span>
        </button>

        <!-- System State Badge -->
        <div class="flex items-center gap-2">
          <span class="text-xs font-semibold text-gray-500 dark:text-gray-400">{{ t('ui.hypervisorDashboardSection.degradationState') }}</span>
          <span
            class="inline-flex items-center rounded-md px-2.5 py-1 text-xs font-bold uppercase tracking-wider"
            :class="stateBadgeClass"
          >
            {{ healthData?.system_state || 'NORMAL' }}
          </span>
        </div>
      </div>
    </div>

    <!-- ========================================================================= -->
    <!-- STANDARD ENGINE METRICS CARDS -->
    <!-- ========================================================================= -->
    <div v-if="activeEngine === 'standard'" class="grid grid-cols-2 gap-4 lg:grid-cols-5">
      <!-- Total Nodes -->
      <div class="rounded-xl border border-gray-200 bg-gray-50/50 p-4 dark:border-gray-700 dark:bg-gray-800/40">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('ui.hypervisorDashboardSection.poolNodes') }}</span>
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
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('ui.hypervisorDashboardSection.activeSessions') }}</span>
          <Icon name="chart" size="sm" class="text-indigo-500" />
        </div>
        <p class="mt-2 text-2xl font-black text-indigo-600 dark:text-indigo-400">
          {{ globalStats?.active_sessions_count || 0 }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('ui.hypervisorDashboardSection.virtualRebindingActive') }}
        </p>
      </div>

      <!-- Total Tokens Handled -->
      <div class="rounded-xl border border-gray-200 bg-gray-50/50 p-4 dark:border-gray-700 dark:bg-gray-800/40">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('ui.hypervisorDashboardSection.inputOutputTokens') }}</span>
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
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('ui.hypervisorDashboardSection.usageCostUnits') }}</span>
          <Icon name="cube" size="sm" class="text-emerald-500" />
        </div>
        <p class="mt-2 text-2xl font-black text-emerald-600 dark:text-emerald-400">
          {{ globalStats?.total_cost_units || 0 }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('ui.hypervisorDashboardSection.pooledCapacityUnits') }}
        </p>
      </div>

      <!-- 24h Rebinding Analytics -->
      <div class="rounded-xl border border-gray-200 bg-gray-50/50 p-4 dark:border-gray-700 dark:bg-gray-800/40">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">24h Rebindings</span>
          <Icon name="sync" size="sm" class="text-purple-500" />
        </div>
        <p class="mt-2 text-2xl font-black text-purple-600 dark:text-purple-400">
          {{ analyticsData?.rebinding_24h_count || 0 }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          Total Events: {{ analyticsData?.total_events_24h || 0 }}
        </p>
      </div>
    </div>

    <!-- ========================================================================= -->
    <!-- YUNWU HYPERVISOR ENGINE METRICS CARDS (BALANCE & NODES) -->
    <!-- ========================================================================= -->
    <div v-else class="grid grid-cols-2 gap-4 lg:grid-cols-5">
      <!-- Total Available Yunwu Balance -->
      <div class="rounded-xl border border-amber-200 bg-amber-50/40 p-4 dark:border-amber-900/40 dark:bg-amber-950/20">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-amber-800 dark:text-amber-300">{{ t('ui.hypervisorDashboardSection.totalYunwuBalance') }}</span>
          <Icon name="bolt" size="sm" class="text-amber-600 dark:text-amber-400" />
        </div>
        <p class="mt-2 text-2xl font-black text-amber-700 dark:text-amber-300 font-mono flex items-center gap-1">
          <span class="text-amber-500 font-bold">⚡</span>
          <span>{{ formatYunwuCredits(yunwuTotalAvailable) }}</span>
        </p>
        <p class="mt-1 text-xs text-amber-700/80 dark:text-amber-400/80">
          {{ yunwuNodes.length }} Active {{ yunwuNodes.length === 1 ? 'Channel' : 'Channels' }}
        </p>
      </div>

      <!-- Allocatable Balance (After Safety Reserve) -->
      <div class="rounded-xl border border-indigo-200 bg-indigo-50/50 p-4 dark:border-indigo-900/50 dark:bg-indigo-950/20">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-indigo-800 dark:text-indigo-300">{{ t('ui.hypervisorDashboardSection.allocatableBalance') }}</span>
          <Icon name="cube" size="sm" class="text-indigo-600 dark:text-indigo-400" />
        </div>
        <p class="mt-2 text-2xl font-black text-indigo-700 dark:text-indigo-300 font-mono flex items-center gap-1">
          <span class="text-indigo-500 font-bold">⚡</span>
          <span>{{ formatYunwuCredits(yunwuTotalAllocatable) }}</span>
        </p>
        <p class="mt-1 text-xs text-indigo-600 dark:text-indigo-400">
          {{ t('ui.hypervisorDashboardSection.afterSafetyReserveHold') }}
        </p>
      </div>

      <!-- Yunwu Nodes Pool -->
      <div class="rounded-xl border border-gray-200 bg-gray-50/50 p-4 dark:border-gray-700 dark:bg-gray-800/40">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('ui.hypervisorDashboardSection.yunwuNodes') }}</span>
          <span class="rounded bg-purple-100 px-1.5 py-0.5 text-[10px] font-bold text-purple-700 dark:bg-purple-900/50 dark:text-purple-300">
            {{ healthData?.healthy_nodes || yunwuNodes.filter(n => n.state === 'healthy').length }} Healthy
          </span>
        </div>
        <p class="mt-2 text-2xl font-black text-gray-900 dark:text-white">
          {{ healthData?.total_nodes || yunwuNodes.length }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          Cooldown: {{ healthData?.cooldown_nodes || yunwuNodes.filter(n => n.state === 'cooldown').length }}
        </p>
      </div>

      <!-- 24h Rebindings -->
      <div class="rounded-xl border border-gray-200 bg-gray-50/50 p-4 dark:border-gray-700 dark:bg-gray-800/40">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">24h Failovers</span>
          <Icon name="sync" size="sm" class="text-violet-500" />
        </div>
        <p class="mt-2 text-2xl font-black text-violet-600 dark:text-violet-400">
          {{ healthData?.rebinding_24h_cnt || 0 }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('ui.hypervisorDashboardSection.zeroDowntimeRebindings') }}
        </p>
      </div>

      <!-- Sync Freshness -->
      <div class="rounded-xl border border-gray-200 bg-gray-50/50 p-4 dark:border-gray-700 dark:bg-gray-800/40">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('ui.hypervisorDashboardSection.balanceHealth') }}</span>
          <Icon name="clock" size="sm" class="text-amber-500" />
        </div>
        <div class="mt-2 flex items-baseline gap-1">
          <span class="text-2xl font-black text-gray-900 dark:text-white">
            {{ yunwuFreshCount }}
          </span>
          <span class="text-sm font-semibold text-gray-400">/ {{ yunwuNodes.length }} Fresh</span>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('ui.hypervisorDashboardSection.backgroundAutoSync1m') }}
        </p>
      </div>
    </div>

    <!-- ========================================================================= -->
    <!-- STANDARD NODE METRICS TABLE -->
    <!-- ========================================================================= -->
    <div v-if="activeEngine === 'standard'" class="space-y-3">
      <div class="flex items-center justify-between">
        <h4 class="text-sm font-bold text-gray-900 dark:text-white flex items-center gap-2">
          <span>{{ t('ui.hypervisorDashboardSection.executionNodeHealthLoad') }}</span>
          <span class="rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 dark:bg-gray-800 dark:text-gray-300">
            {{ nodeMetrics.length }} registered
          </span>
        </h4>
      </div>

      <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
        <table class="w-full text-left text-xs text-gray-600 dark:text-gray-300">
          <thead class="bg-gray-100/70 text-xs font-semibold uppercase tracking-wider text-gray-700 dark:bg-gray-800/60 dark:text-gray-300">
            <tr>
              <th class="px-4 py-3">{{ t('ui.hypervisorDashboardSection.nodeAccount') }}</th>
              <th class="px-4 py-3">{{ t('ui.hypervisorDashboardSection.platform') }}</th>
              <th class="px-4 py-3">{{ t('ui.hypervisorDashboardSection.status') }}</th>
              <th class="px-4 py-3">{{ t('ui.hypervisorDashboardSection.successRate') }}</th>
              <th class="px-4 py-3">{{ t('ui.hypervisorDashboardSection.avgLatency') }}</th>
              <th class="px-4 py-3">{{ t('ui.hypervisorDashboardSection.recent429Timeout') }}</th>
              <th class="px-4 py-3">{{ t('ui.hypervisorDashboardSection.cooldownUntil') }}</th>
              <th class="px-4 py-3 text-right">{{ t('ui.hypervisorDashboardSection.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 dark:divide-gray-700">
            <tr v-if="nodeMetrics.length === 0">
              <td colspan="8" class="py-8 text-center text-gray-500 dark:text-gray-400">
                {{ t('ui.hypervisorDashboardSection.noactiveexecutionnodes') }}
              </td>
            </tr>
            <tr
              v-for="node in nodeMetrics"
              :key="node.node_id"
              class="hover:bg-gray-50/80 dark:hover:bg-gray-800/50 transition-colors"
            >
              <!-- Node Name / ID -->
              <td class="px-4 py-3 font-semibold text-gray-900 dark:text-white">
                <div>{{ node.account_name || (t('ui.hypervisorDashboardSection.nodePrefix') + node.node_id) }}</div>
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
                    :title="t('ui.hypervisorDashboardSection.putin10minute')"
                  >
                    {{ t('ui.hypervisorDashboardSection.cooldown') }}
                  </button>
                  <button
                    v-if="node.state === 'unavailable' || node.state === 'cooldown'"
                    @click="handleEnable(node.node_id)"
                    class="rounded px-2 py-1 text-[11px] font-medium bg-emerald-50 text-emerald-700 hover:bg-emerald-100 dark:bg-emerald-900/30 dark:text-emerald-300 dark:hover:bg-emerald-900/50"
                  >
                    {{ t('ui.hypervisorDashboardSection.enable') }}
                  </button>
                  <button
                    v-else
                    @click="handleDisable(node.node_id)"
                    class="rounded px-2 py-1 text-[11px] font-medium bg-rose-50 text-rose-700 hover:bg-rose-100 dark:bg-rose-900/30 dark:text-rose-300 dark:hover:bg-rose-900/50"
                  >
                    {{ t('ui.hypervisorDashboardSection.disable') }}
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- ========================================================================= -->
    <!-- YUNWU NODE BALANCE & HEALTH TABLE -->
    <!-- ========================================================================= -->
    <div v-else class="space-y-3">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex flex-wrap items-center gap-3">
          <h4 class="text-sm font-bold text-gray-900 dark:text-white flex items-center gap-2">
            <span>{{ t('ui.hypervisorDashboardSection.yunwuAPIChannelBalances') }}</span>
            <span class="rounded bg-purple-100 px-2 py-0.5 text-xs font-semibold text-purple-700 dark:bg-purple-900/50 dark:text-purple-300">
              {{ yunwuNodes.length }} channels
            </span>
          </h4>

          <!-- Group Filter Dropdown -->
          <div class="flex items-center gap-1.5 text-xs">
            <span class="text-gray-500 dark:text-gray-400 font-medium">{{ t('ui.hypervisorDashboardSection.group') }}</span>
            <select
              v-model="selectedGroupFilter"
              @change="handleGroupFilterChange"
              class="rounded-lg border border-gray-300 bg-white px-2.5 py-1 text-xs text-gray-700 focus:border-purple-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-200"
            >
              <option value="">{{ t('ui.hypervisorDashboardSection.allGroups') }}</option>
              <option v-for="g in availableGroups" :key="g.id" :value="g.id">
                {{ g.name }}
              </option>
            </select>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <!-- Sync Filtered Group Button -->
          <button
            v-if="selectedGroupFilter"
            @click="handleSyncFilteredGroup"
            :disabled="syncingAll || loading"
            class="btn btn-secondary text-xs flex items-center gap-1 bg-purple-50 text-purple-700 hover:bg-purple-100 dark:bg-purple-950/40 dark:text-purple-300 border-purple-200 dark:border-purple-800"
            :title="t('ui.hypervisorDashboardSection.syncbalancesforall')"
          >
            <Icon name="bolt" size="xs" :class="{ 'animate-spin': syncingAll }" />
            <span>{{ t('ui.hypervisorDashboardSection.syncGroupBalances') }}</span>
          </button>
        </div>
      </div>

      <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
        <table class="w-full text-left text-xs text-gray-600 dark:text-gray-300">
          <thead class="bg-gray-100/70 text-xs font-semibold uppercase tracking-wider text-gray-700 dark:bg-gray-800/60 dark:text-gray-300">
            <tr>
              <th class="px-4 py-3">{{ t('ui.hypervisorDashboardSection.nodeAccount2') }}</th>
              <th class="px-4 py-3">Available Balance (⚡)</th>
              <th class="px-4 py-3">Allocatable Balance (⚡)</th>
              <th class="px-4 py-3">{{ t('ui.hypervisorDashboardSection.reserve') }}</th>
              <th class="px-4 py-3">{{ t('ui.hypervisorDashboardSection.nodeState') }}</th>
              <th class="px-4 py-3">{{ t('ui.hypervisorDashboardSection.lastSync') }}</th>
              <th class="px-4 py-3 text-right">{{ t('ui.hypervisorDashboardSection.actions2') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 dark:divide-gray-700">
            <tr v-if="yunwuNodes.length === 0">
              <td colspan="7" class="py-8 text-center text-gray-500 dark:text-gray-400">
                {{ t('ui.hypervisorDashboardSection.noYunwuAPIchannel') }}
              </td>
            </tr>
            <tr
              v-for="node in yunwuNodes"
              :key="node.node_id"
              class="hover:bg-gray-50/80 dark:hover:bg-gray-800/50 transition-colors"
            >
              <!-- Node Name / Account -->
              <td class="px-4 py-3 font-semibold text-gray-900 dark:text-white">
                <div class="flex flex-wrap items-center gap-1.5">
                  <span>{{ node.account_name || (t('ui.hypervisorDashboardSection.yunwuNodePrefix') + node.node_id) }}</span>
                  <span
                    v-if="node.provider"
                    class="rounded px-1.5 py-0.5 text-[10px] font-mono font-semibold uppercase tracking-wider"
                    :class="getProviderBadgeClass(node.provider)"
                  >
                    {{ node.provider }}
                  </span>
                  <span class="rounded bg-purple-100 px-1.5 py-0.2 text-[10px] font-mono text-purple-700 dark:bg-purple-900/40 dark:text-purple-300">
                    {{ node.type || 'api_key' }}
                  </span>
                </div>
                <div class="flex flex-wrap items-center gap-2 mt-0.5 text-[10px] text-gray-400">
                  <span class="font-mono">ID: {{ node.node_id }}</span>
                  <span v-if="node.groups && node.groups.length > 0" class="flex items-center gap-1">
                    <span
                      v-for="grp in node.groups"
                      :key="grp.id"
                      class="rounded bg-gray-100 px-1.5 py-0.2 text-[9px] text-gray-600 dark:bg-gray-700 dark:text-gray-300"
                    >
                      {{ grp.name }}
                    </span>
                  </span>
                  <span v-if="node.base_url" class="truncate max-w-[200px]" :title="node.base_url">| {{ node.base_url }}</span>
                </div>
              </td>

              <!-- Available Balance -->
              <td class="px-4 py-3 font-mono font-bold text-gray-900 dark:text-white">
                <div class="flex items-center gap-2">
                  <span class="text-sm text-amber-600 dark:text-amber-400 flex items-center gap-0.5">
                    <span class="text-amber-500 font-bold">⚡</span>
                    <span>{{ formatYunwuCredits(node.total_available_usd) }}</span>
                  </span>
                  <span
                    class="rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider"
                    :class="getBalanceStatusClass(node.balance_status)"
                  >
                    {{ node.balance_status || 'unknown' }}
                  </span>
                </div>
              </td>

              <!-- Allocatable Balance -->
              <td class="px-4 py-3 font-mono font-semibold text-indigo-600 dark:text-indigo-400">
                <span class="flex items-center gap-0.5">
                  <span class="text-indigo-500 font-bold">⚡</span>
                  <span>{{ formatYunwuCredits(node.allocatable_usd !== undefined ? node.allocatable_usd : node.total_available_usd * (1 - (node.reserve_percent || 0)/100)) }}</span>
                </span>
              </td>

              <!-- Reserve % -->
              <td class="px-4 py-3 font-mono">
                <span class="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] font-medium text-gray-700 dark:bg-gray-800 dark:text-gray-300">
                  {{ node.reserve_percent || 0 }}%
                </span>
              </td>

              <!-- Node State Badge -->
              <td class="px-4 py-3">
                <span
                  class="inline-flex items-center rounded-full px-2.5 py-0.5 text-[11px] font-semibold"
                  :class="getNodeStateClass(node.state)"
                >
                  {{ node.state || 'healthy' }}
                </span>
              </td>

              <!-- Last Synchronized -->
              <td class="px-4 py-3 font-mono text-xs">
                <div v-if="node.balance_synchronized_at" :title="'Source: ' + (node.balance_source || 'unknown') + '\nExact: ' + node.balance_synchronized_at">
                  <div class="text-gray-700 dark:text-gray-300">{{ formatRelativeTime(node.balance_synchronized_at) }}</div>
                  <div class="text-[10px] text-gray-400">{{ formatDate(node.balance_synchronized_at) }}</div>
                </div>
                <span v-else class="text-gray-400">{{ t('ui.hypervisorDashboardSection.never') }}</span>
              </td>

              <!-- Actions -->
              <td class="px-4 py-3 text-right">
                <div class="flex items-center justify-end gap-1.5">
                  <!-- Sync Balance Button -->
                  <button
                    @click="handleSyncNodeBalance(node.node_id)"
                    class="rounded px-2.5 py-1 text-[11px] font-medium bg-purple-50 text-purple-700 hover:bg-purple-100 dark:bg-purple-900/30 dark:text-purple-300 dark:hover:bg-purple-900/50 flex items-center gap-1"
                    :disabled="syncingNodes[node.node_id]"
                    :title="t('ui.hypervisorDashboardSection.syncbalancefromupstream')"
                  >
                    <Icon name="sync" size="xs" :class="{ 'animate-spin': syncingNodes[node.node_id] }" />
                    <span>{{ syncingNodes[node.node_id] ? 'Syncing...' : 'Sync' }}</span>
                  </button>

                  <button
                    @click="handleCooldown(node.node_id)"
                    class="rounded px-2 py-1 text-[11px] font-medium bg-amber-50 text-amber-700 hover:bg-amber-100 dark:bg-amber-900/30 dark:text-amber-300 dark:hover:bg-amber-900/50"
                    :title="t('ui.hypervisorDashboardSection.putin10minute2')"
                  >
                    {{ t('ui.hypervisorDashboardSection.cooldown2') }}
                  </button>

                  <button
                    v-if="node.state === 'unavailable' || node.state === 'cooldown'"
                    @click="handleEnable(node.node_id)"
                    class="rounded px-2 py-1 text-[11px] font-medium bg-emerald-50 text-emerald-700 hover:bg-emerald-100 dark:bg-emerald-900/30 dark:text-emerald-300 dark:hover:bg-emerald-900/50"
                  >
                    {{ t('ui.hypervisorDashboardSection.enable2') }}
                  </button>
                  <button
                    v-else
                    @click="handleDisable(node.node_id)"
                    class="rounded px-2 py-1 text-[11px] font-medium bg-rose-50 text-rose-700 hover:bg-rose-100 dark:bg-rose-900/30 dark:text-rose-300 dark:hover:bg-rose-900/50"
                  >
                    {{ t('ui.hypervisorDashboardSection.disable2') }}
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
import { ref, computed, watch, onMounted, onUnmounted, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AshHealthResponse, AshGlobalStats, AshNodeMetric, AshAnalytics, YunwuNodeItem } from '@/api/admin/ash'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()

const activeEngine = ref<'standard' | 'yunwu'>('standard')
const loading = ref(false)
const syncingAll = ref(false)
const syncingNodes = reactive<Record<string, boolean>>({})

const healthData = ref<AshHealthResponse | null>(null)
const globalStats = ref<AshGlobalStats | null>(null)
const nodeMetrics = ref<AshNodeMetric[]>([])
const yunwuNodes = ref<YunwuNodeItem[]>([])
const analyticsData = ref<AshAnalytics | null>(null)
let refreshTimer: ReturnType<typeof setInterval> | null = null

// Aggregate totals for Yunwu Balance
const yunwuTotalAvailable = computed(() => {
  if (healthData.value?.total_available_usd !== undefined && healthData.value.total_available_usd > 0) {
    return healthData.value.total_available_usd
  }
  return yunwuNodes.value.reduce((sum, n) => sum + (n.total_available_usd || 0), 0)
})

const yunwuTotalAllocatable = computed(() => {
  if (healthData.value?.total_allocatable_usd !== undefined && healthData.value.total_allocatable_usd > 0) {
    return healthData.value.total_allocatable_usd
  }
  return yunwuNodes.value.reduce((sum, n) => {
    const alloc = n.allocatable_usd !== undefined ? n.allocatable_usd : (n.total_available_usd || 0) * (1 - (n.reserve_percent || 0)/100)
    return sum + alloc
  }, 0)
})

const yunwuFreshCount = computed(() => {
  return yunwuNodes.value.filter(n => n.balance_status === 'fresh').length
})

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

const getBalanceStatusClass = (status?: string) => {
  switch (status) {
    case 'fresh':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
    case 'stale':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
    default:
      return 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'
  }
}

const formatTokens = (val: number) => {
  if (!val) return '0'
  if (val >= 1000000) return (val / 1000000).toFixed(2) + 'M'
  if (val >= 1000) return (val / 1000).toFixed(1) + 'K'
  return val.toString()
}

const formatYunwuCredits = (val: number | undefined) => {
  if (val === undefined || val === null) return '0.00'
  return Number(val).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
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

const formatRelativeTime = (dateStr?: string) => {
  if (!dateStr) return '-'
  try {
    const d = new Date(dateStr)
    const diffSeconds = Math.floor((Date.now() - d.getTime()) / 1000)
    if (diffSeconds < 5) return t('common.time.justNow')
    if (diffSeconds < 60) return t('common.time.justNow')
    if (diffSeconds < 3600) return t('common.time.minutesAgo', { n: Math.floor(diffSeconds / 60) })
    if (diffSeconds < 86400) return t('common.time.hoursAgo', { n: Math.floor(diffSeconds / 3600) })
    return t('common.time.daysAgo', { n: Math.floor(diffSeconds / 86400) })
  } catch {
    return dateStr
  }
}

const selectedGroupFilter = ref<string | number>('')

const availableGroups = computed(() => {
  const map = new Map<number, string>()
  for (const node of yunwuNodes.value) {
    if (node.groups) {
      for (const g of node.groups) {
        map.set(g.id, g.name)
      }
    }
  }
  return Array.from(map.entries()).map(([id, name]) => ({ id, name }))
})

const getProviderBadgeClass = (provider?: string) => {
  switch (provider?.toLowerCase()) {
    case 'openlux':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300 border border-blue-200 dark:border-blue-800'
    case 'portdan':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
    case 'api-top':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300 border border-amber-200 dark:border-amber-800'
    case 'yuanqiaoyun':
      return 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800'
    case 'x-llm':
      return 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300 border border-purple-200 dark:border-purple-800'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300'
  }
}

const handleGroupFilterChange = () => {
  loadAshData()
}

const loadAshData = async () => {
  loading.value = true
  try {
    const groupID = selectedGroupFilter.value ? Number(selectedGroupFilter.value) : undefined
    if (activeEngine.value === 'yunwu') {
      const [healthRes, ywNodes] = await Promise.all([
        adminAPI.ash.getAshYunwuHealth().catch(() => null),
        adminAPI.ash.getAshYunwuNodes(groupID).catch(() => [])
      ])
      healthData.value = healthRes
      yunwuNodes.value = ywNodes || []
    } else {
      const [healthRes, statsRes, nodesRes, analyticsRes, ywNodes] = await Promise.all([
        adminAPI.ash.getAshHealth().catch(() => null),
        adminAPI.ash.getAshGlobalStats().catch(() => null),
        adminAPI.ash.getAshNodeMetrics().catch(() => []),
        adminAPI.ash.getAshAnalytics().catch(() => null),
        adminAPI.ash.getAshYunwuNodes(groupID).catch(() => [])
      ])
      healthData.value = healthRes
      globalStats.value = statsRes
      nodeMetrics.value = (nodesRes || []).filter(n => n.provider !== 'yunwu')
      analyticsData.value = analyticsRes
      yunwuNodes.value = ywNodes || []
    }
  } catch (err) {
    console.error('Failed to load ASH hypervisor stats:', err)
  } finally {
    loading.value = false
  }
}

watch(activeEngine, loadAshData)

const handleSyncNodeBalance = async (nodeID: string) => {
  syncingNodes[nodeID] = true
  try {
    await adminAPI.ash.syncAshYunwuNode(nodeID)
    await loadAshData()
  } catch (err) {
    console.error('Failed to sync node balance:', err)
  } finally {
    syncingNodes[nodeID] = false
  }
}

const handleSyncFilteredGroup = async () => {
  if (!selectedGroupFilter.value) return
  syncingAll.value = true
  try {
    await adminAPI.ash.syncAshYunwuGroup(selectedGroupFilter.value)
    await loadAshData()
  } catch (err) {
    console.error('Failed to sync group balances:', err)
  } finally {
    syncingAll.value = false
  }
}

const handleSyncAllYunwu = async () => {
  syncingAll.value = true
  try {
    const groupID = selectedGroupFilter.value ? Number(selectedGroupFilter.value) : undefined
    await adminAPI.ash.syncAllAshYunwuNodes(groupID)
    await loadAshData()
  } catch (err) {
    console.error('Failed to sync all Yunwu nodes:', err)
  } finally {
    syncingAll.value = false
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

<style scoped>
</style>
