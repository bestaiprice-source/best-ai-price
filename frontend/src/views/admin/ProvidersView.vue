<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Header -->
      <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
            AI Providers
          </h1>
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <button
            @click="handleSyncAll"
            :disabled="isSyncingAll"
            class="inline-flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 shadow-sm transition-all hover:bg-gray-50 hover:shadow disabled:opacity-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
          >
            <Icon name="sync" size="sm" :class="{ 'animate-spin': isSyncingAll }" />
            <span>{{ isSyncingAll ? 'Syncing...' : 'Sync All Balances' }}</span>
          </button>
          <button
            @click="openCreateModal"
            class="inline-flex items-center gap-2 rounded-xl bg-primary-600 px-4 py-2.5 text-sm font-medium text-white shadow-sm transition-all hover:bg-primary-500 hover:shadow-md dark:bg-primary-500 dark:hover:bg-primary-400"
          >
            <Icon name="plus" size="sm" />
            <span>Add Provider</span>
          </button>
          <router-link
            to="/admin/vendors"
            class="inline-flex items-center gap-2 rounded-xl border border-indigo-200 bg-indigo-50 px-4 py-2.5 text-sm font-medium text-indigo-700 shadow-sm transition-all hover:bg-indigo-100 hover:shadow dark:border-indigo-800 dark:bg-indigo-950/40 dark:text-indigo-300 dark:hover:bg-indigo-900/50"
          >
            <Icon name="lightbulb" size="sm" />
            <span>Cheaper Alternatives</span>
          </router-link>
          <router-link
            to="/admin/accounts"
            class="inline-flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 shadow-sm transition-all hover:bg-gray-50 hover:shadow dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700"
          >
            <Icon name="server" size="sm" />
            <span>Manage Nodes</span>
          </router-link>
        </div>
      </div>

      <!-- KPI Summary Cards -->
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-5">
        <!-- Total Providers -->
        <div
          @click="resetFilters"
          title="Reset filters and show all"
          class="relative cursor-pointer overflow-hidden rounded-2xl border border-gray-200/80 bg-white/70 p-5 shadow-sm backdrop-blur-sm transition-transform hover:scale-[1.01] dark:border-dark-700/80 dark:bg-dark-800/70"
        >
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">Total Providers</span>
            <div class="rounded-xl bg-blue-500/10 p-2.5 text-blue-600 dark:bg-blue-500/20 dark:text-blue-400">
              <Icon name="grid" size="sm" />
            </div>
          </div>
          <p class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ summaryStats.total_providers }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">Connected upstream services</p>
        </div>

        <!-- Active Providers -->
        <div
          @click="filters.status = 'active'; filters.state = ''; fetchData()"
          title="Filter: active only"
          class="relative cursor-pointer overflow-hidden rounded-2xl border border-gray-200/80 bg-white/70 p-5 shadow-sm backdrop-blur-sm transition-transform hover:scale-[1.01] dark:border-dark-700/80 dark:bg-dark-800/70"
        >
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">Active</span>
            <div class="rounded-xl bg-emerald-500/10 p-2.5 text-emerald-600 dark:bg-emerald-500/20 dark:text-emerald-400">
              <Icon name="check" size="sm" />
            </div>
          </div>
          <p class="mt-2 text-2xl font-bold text-emerald-600 dark:text-emerald-400">{{ summaryStats.active_providers }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">Ready for traffic</p>
        </div>

        <!-- Low Balance Alert -->
        <div
          @click="filters.state = 'Low balance'; filters.status = ''; fetchData()"
          title="Filter: low balance"
          class="relative cursor-pointer overflow-hidden rounded-2xl border border-gray-200/80 bg-white/70 p-5 shadow-sm backdrop-blur-sm transition-transform hover:scale-[1.01] dark:border-dark-700/80 dark:bg-dark-800/70"
          :class="{ 'ring-2 ring-red-500/30': summaryStats.low_balance_providers > 0 }"
        >
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">Low Balance</span>
            <div :class="[
              'rounded-xl p-2.5',
              summaryStats.low_balance_providers > 0
                ? 'bg-rose-500/15 text-rose-600 dark:bg-rose-500/25 dark:text-rose-400 animate-pulse'
                : 'bg-gray-100 text-gray-400 dark:bg-dark-700 dark:text-gray-500'
            ]">
              <Icon name="lightbulb" size="sm" />
            </div>
          </div>
          <p class="mt-2 text-2xl font-bold" :class="summaryStats.low_balance_providers > 0 ? 'text-rose-600 dark:text-rose-400' : 'text-gray-900 dark:text-white'">
            {{ summaryStats.low_balance_providers }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">Below min_balance threshold</p>
        </div>

        <!-- Total Balance USD -->
        <div class="relative overflow-hidden rounded-2xl border border-gray-200/80 bg-white/70 p-5 shadow-sm backdrop-blur-sm dark:border-dark-700/80 dark:bg-dark-800/70">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">Total Balance</span>
            <div class="rounded-xl bg-violet-500/10 p-2.5 text-violet-600 dark:bg-violet-500/20 dark:text-violet-400">
              <span class="text-base font-bold">$</span>
            </div>
          </div>
          <p class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">${{ formatCurrency(summaryStats.total_balance_usd) }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">Available across accounts</p>
        </div>

        <!-- Total Spend USD -->
        <div class="relative overflow-hidden rounded-2xl border border-gray-200/80 bg-white/70 p-5 shadow-sm backdrop-blur-sm dark:border-dark-700/80 dark:bg-dark-800/70">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">Total Spend</span>
            <div class="rounded-xl bg-amber-500/10 p-2.5 text-amber-600 dark:bg-amber-500/20 dark:text-amber-400">
              <Icon name="chart" size="sm" />
            </div>
          </div>
          <p class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">${{ formatCurrency(summaryStats.total_spend_usd) }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">Used during current period</p>
        </div>
      </div>

      <!-- Filters & Search Toolbar -->
      <div class="flex flex-col gap-3 rounded-2xl border border-gray-200 bg-white p-4 shadow-sm sm:flex-row sm:items-center sm:justify-between dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-1 flex-wrap items-center gap-3">
          <!-- Search -->
          <div class="relative min-w-[240px] flex-1 sm:max-w-xs">
            <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input
              v-model="filters.search"
              @input="handleSearchInput"
              type="text"
              placeholder="Search by name, domain..."
              class="w-full rounded-xl border border-gray-200 bg-gray-50/50 py-2 pl-9 pr-3 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:bg-white focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-700/50 dark:text-white dark:focus:bg-dark-700"
            />
          </div>

          <!-- Status Filter -->
          <select
            v-model="filters.status"
            @change="fetchData"
            class="rounded-xl border border-gray-200 bg-white py-2 pl-3 pr-8 text-sm text-gray-700 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200"
          >
            <option value="">All Statuses</option>
            <option value="active">Active Only</option>
            <option value="disabled">Disabled</option>
          </select>

          <!-- State Filter -->
          <select
            v-model="filters.state"
            @change="fetchData"
            class="rounded-xl border border-gray-200 bg-white py-2 pl-3 pr-8 text-sm text-gray-700 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200"
          >
            <option value="">All States</option>
            <option value="Healthy">Healthy</option>
            <option value="Degraded">Degraded</option>
            <option value="Down">Down</option>
            <option value="Low balance">Low balance</option>
            <option value="Invalid key">Invalid key</option>
            <option value="Disabled">Disabled</option>
          </select>
        </div>

        <button
          @click="fetchData"
          class="inline-flex items-center gap-1.5 rounded-xl border border-gray-200 px-3 py-2 text-sm font-medium text-gray-600 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
        >
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': isLoading }" />
          <span>Refresh</span>
        </button>
      </div>

      <!-- Providers Table -->
      <div class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-gray-200 bg-gray-50/75 text-xs font-semibold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:bg-dark-900/50 dark:text-gray-400">
              <tr>
                <th class="px-5 py-3.5">Provider</th>
                <th class="px-5 py-3.5">State</th>
                <th class="px-5 py-3.5">Balance</th>
                <th class="px-5 py-3.5">Min Threshold</th>
                <th class="px-5 py-3.5">Metrics</th>
                <th class="px-5 py-3.5">Models</th>
                <th class="px-5 py-3.5 text-center">Status</th>
                <th class="px-5 py-3.5 text-right">Actions</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700/60">
              <tr v-if="isLoading && providers.length === 0">
                <td colspan="8" class="px-5 py-12 text-center text-gray-500 dark:text-gray-400">
                  <div class="flex flex-col items-center justify-center gap-2">
                    <Icon name="sync" size="lg" class="animate-spin text-primary-500" />
                    <span>Loading providers list...</span>
                  </div>
                </td>
              </tr>
              <tr v-else-if="providers.length === 0">
                <td colspan="8" class="px-5 py-12 text-center text-gray-500 dark:text-gray-400">
                  <div class="flex flex-col items-center justify-center gap-2">
                    <Icon name="grid" size="lg" class="text-gray-300 dark:text-gray-600" />
                    <span class="text-base font-medium">No Yunwu providers found</span>
                    <span class="text-xs text-gray-400 max-w-md text-center">
                      This section displays upstream providers for the Yunwu group (OpenLux, Portdan, Yuanqiaoyun, API-Top, X-LLM, etc.). Official accounts (Claude, OpenAI, Gemini) are configured under Nodes.
                    </span>
                    <div class="mt-2 flex items-center gap-3">
                      <router-link
                        to="/admin/accounts"
                        class="inline-flex items-center gap-1.5 rounded-lg border border-gray-200 bg-white px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-750"
                      >
                        <Icon name="server" size="sm" />
                        Go to Nodes
                      </router-link>
                      <button
                        v-if="filters.search || filters.status || filters.state"
                        @click="resetFilters"
                        class="text-xs text-primary-500 hover:underline"
                      >
                        Reset filters
                      </button>
                    </div>
                  </div>
                </td>
              </tr>
              <tr
                v-for="provider in providers"
                :key="provider.id"
                class="transition-colors hover:bg-gray-50/60 dark:hover:bg-dark-700/40"
              >
                <!-- Provider Name & Domain -->
                <td class="px-5 py-4">
                  <div class="flex items-center gap-3">
                    <div class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl font-bold uppercase text-white shadow-sm"
                         :style="{ backgroundColor: getProviderAvatarColor(provider.provider_name) }">
                      {{ provider.provider_name.slice(0, 2) }}
                    </div>
                    <div class="min-w-0">
                      <div class="flex items-center gap-2">
                        <span class="font-semibold text-gray-900 dark:text-white">{{ provider.provider_name }}</span>
                        <!-- Protocol Badge -->
                        <span class="inline-flex items-center rounded-md px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider"
                              :class="getProtocolBadgeClass(provider.protocol)">
                          {{ provider.protocol }}
                        </span>
                      </div>
                      <div class="mt-0.5 flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
                        <a
                          v-if="provider.base_url"
                          :href="provider.base_url"
                          target="_blank"
                          rel="noopener noreferrer"
                          class="truncate hover:text-primary-600 dark:hover:text-primary-400"
                          :title="provider.base_url"
                        >
                          {{ provider.domain || provider.base_url }}
                        </a>
                        <span v-else>{{ provider.domain || '—' }}</span>
                      </div>
                    </div>
                  </div>
                </td>

                <!-- State Badge -->
                <td class="px-5 py-4 whitespace-nowrap">
                  <span
                    class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium"
                    :class="getStateBadgeClass(provider.state)"
                  >
                    <span class="h-1.5 w-1.5 rounded-full" :class="getStateDotClass(provider.state)"></span>
                    {{ provider.state }}
                  </span>
                </td>

                <!-- Balance -->
                <td class="px-5 py-4 whitespace-nowrap">
                  <div>
                    <div class="flex items-center gap-1.5">
                      <span class="font-bold text-gray-900 dark:text-white"
                            :class="{ 'text-rose-600 dark:text-rose-400': provider.balance_usd <= provider.min_balance_usd && provider.status === 'active' }">
                        ${{ formatCurrency(provider.balance_usd) }}
                      </span>
                      <span v-if="provider.balance_usd <= provider.min_balance_usd && provider.status === 'active'"
                            class="inline-flex items-center rounded bg-rose-50 px-1 py-0.2 text-[10px] font-semibold text-rose-600 dark:bg-rose-950/40 dark:text-rose-400"
                            title="Balance is below the minimum threshold">
                        Low
                      </span>
                    </div>
                    <div class="mt-0.5 text-[11px] text-gray-400 dark:text-gray-500">
                      {{ formatTimeAgo(provider.last_synced_at) }}
                      <span v-if="provider.balance_source" class="opacity-70">({{ provider.balance_source }})</span>
                    </div>
                  </div>
                </td>

                <!-- Min Balance -->
                <td class="px-5 py-4 whitespace-nowrap">
                  <span class="font-medium text-gray-700 dark:text-gray-300">
                    ${{ formatCurrency(provider.min_balance_usd) }}
                  </span>
                </td>

                <!-- Runtime Metrics -->
                <td class="px-5 py-4 whitespace-nowrap">
                  <div class="flex items-center gap-3 text-xs">
                    <!-- Latency -->
                    <div class="flex flex-col" :title="'Average latency: ' + (provider.metrics.latency_ms || 0) + ' ms'">
                      <span class="text-[10px] uppercase text-gray-400">Latency</span>
                      <span class="font-medium" :class="getLatencyClass(provider.metrics.latency_ms)">
                        {{ provider.metrics.latency_ms > 0 ? provider.metrics.latency_ms + 'ms' : '—' }}
                      </span>
                    </div>
                    <!-- Error Rate -->
                    <div class="flex flex-col" :title="'Error rate: ' + (provider.metrics.error_rate || 0) + '%'">
                      <span class="text-[10px] uppercase text-gray-400">Errors</span>
                      <span class="font-medium" :class="getErrorRateClass(provider.metrics.error_rate)">
                        {{ provider.metrics.error_rate !== undefined ? provider.metrics.error_rate + '%' : '0%' }}
                      </span>
                    </div>
                    <!-- Requests -->
                    <div class="flex flex-col" :title="'Total requests: ' + (provider.metrics.requests || 0)">
                      <span class="text-[10px] uppercase text-gray-400">Reqs</span>
                      <span class="font-medium text-gray-700 dark:text-gray-300">
                        {{ formatCompactNumber(provider.metrics.requests) }}
                      </span>
                    </div>
                    <!-- Spend -->
                    <div class="flex flex-col" :title="'Spend: $' + (provider.metrics.spend_usd || 0)">
                      <span class="text-[10px] uppercase text-gray-400">Spend</span>
                      <span class="font-medium text-gray-700 dark:text-gray-300">
                        ${{ formatCurrency(provider.metrics.spend_usd) }}
                      </span>
                    </div>
                  </div>
                </td>

                <!-- Models -->
                <td class="px-5 py-4 whitespace-nowrap">
                  <div class="flex items-center gap-1.5">
                    <div class="flex flex-wrap gap-1 max-w-[160px]">
                      <span
                        v-for="family in (provider.supported_model_families || []).slice(0, 3)"
                        :key="family"
                        class="rounded-md bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300"
                      >
                        {{ family }}
                      </span>
                      <span
                        v-if="(provider.supported_model_families || []).length > 3"
                        class="rounded-md bg-gray-100 px-1 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-gray-400"
                      >
                        +{{ provider.supported_model_families.length - 3 }}
                      </span>
                    </div>
                    <button
                      v-if="Object.keys(provider.model_mapping || {}).length > 0"
                      @click="openMappingsModal(provider)"
                      class="text-xs text-primary-600 hover:text-primary-700 dark:text-primary-400"
                      :title="'View model mappings (' + Object.keys(provider.model_mapping).length + ')'"
                    >
                      <Icon name="link" size="xs" />
                    </button>
                  </div>
                </td>

                <!-- Status (Active / Disabled) -->
                <td class="px-5 py-4 whitespace-nowrap text-center">
                  <button
                    @click="toggleProviderStatus(provider)"
                    class="relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500/20"
                    :class="provider.status === 'active' ? 'bg-primary-600 dark:bg-primary-500' : 'bg-gray-200 dark:bg-dark-600'"
                    :title="provider.status === 'active' ? 'Active (click to disable)' : 'Disabled (click to activate)'"
                  >
                    <span
                      class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
                      :class="provider.status === 'active' ? 'translate-x-4' : 'translate-x-0'"
                    />
                  </button>
                </td>

                <!-- Actions -->
                <td class="px-5 py-4 whitespace-nowrap text-right">
                  <div class="flex items-center justify-end gap-1">
                    <!-- Test Connection Button -->
                    <button
                      @click="handleTestConnection(provider)"
                      :disabled="testingProviderId === provider.id"
                      class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-primary-600 disabled:opacity-50 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                      title="Test connection & latency"
                    >
                      <Icon name="play" size="sm" :class="{ 'animate-spin': testingProviderId === provider.id }" />
                    </button>

                    <!-- Sync Balance Button -->
                    <button
                      @click="handleSyncBalance(provider)"
                      :disabled="syncingProviderId === provider.id"
                      class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-emerald-600 disabled:opacity-50 dark:hover:bg-dark-700 dark:hover:text-emerald-400"
                      title="Sync balance"
                    >
                      <Icon name="sync" size="sm" :class="{ 'animate-spin': syncingProviderId === provider.id }" />
                    </button>

                    <!-- Edit Button -->
                    <button
                      @click="openEditModal(provider)"
                      class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-blue-600 dark:hover:bg-dark-700 dark:hover:text-blue-400"
                      title="Edit"
                    >
                      <Icon name="edit" size="sm" />
                    </button>

                    <!-- Delete Button -->
                    <button
                      @click="openDeleteDialog(provider)"
                      class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-rose-50 hover:text-rose-600 dark:hover:bg-rose-950/30 dark:hover:text-rose-400"
                      title="Delete"
                    >
                      <Icon name="trash" size="sm" />
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Pagination -->
        <div v-if="totalPages > 1" class="border-t border-gray-200 px-5 py-3 dark:border-dark-700">
          <Pagination
            :page="currentPage"
            :total="totalProviders"
            :page-size="pageSize"
            @update:page="handlePageChange"
          />
        </div>
      </div>
    </div>

    <!-- Create / Edit Provider Modal -->
    <BaseDialog
      :show="showEditModal"
      :title="editingProvider ? 'Edit Provider' : 'Add New Provider'"
      width="wide"
      @close="closeEditModal"
    >
      <form @submit.prevent="saveProvider" class="space-y-5">
        <!-- Quick Presets -->
        <div v-if="!editingProvider" class="rounded-xl border border-gray-100 bg-gray-50/70 p-3.5 dark:border-dark-700 dark:bg-dark-900/40">
          <label class="block text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-2">
            Quick Provider Preset
          </label>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="preset in presets"
              :key="preset.id"
              type="button"
              @click="applyPreset(preset)"
              class="rounded-lg border px-3 py-1.5 text-xs font-medium transition-all"
              :class="selectedPresetId === preset.id
                ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-950/40 dark:text-primary-300'
                : 'border-gray-200 bg-white text-gray-700 hover:bg-gray-100 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700'"
            >
              {{ preset.name }}
            </button>
          </div>
        </div>

        <!-- Basic Info Row -->
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="block text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-1">
              Provider Name <span class="text-rose-500">*</span>
            </label>
            <input
              v-model="form.provider_name"
              type="text"
              required
              placeholder="e.g. OpenLux, Portdan, API-Top"
              class="w-full rounded-xl border border-gray-200 bg-white px-3.5 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-700 dark:text-white"
            />
          </div>

          <div>
            <label class="block text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-1">
              Domain
            </label>
            <input
              v-model="form.domain"
              type="text"
              placeholder="e.g. api.openlux.ai (auto-extracted from URL if empty)"
              class="w-full rounded-xl border border-gray-200 bg-white px-3.5 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-700 dark:text-white"
            />
          </div>
        </div>

        <!-- Protocol & Base URL -->
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div>
            <label class="block text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-1">
              API Protocol <span class="text-rose-500">*</span>
            </label>
            <select
              v-model="form.protocol"
              required
              class="w-full rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-700 dark:text-white"
            >
              <option value="openai">OpenAI Compatible (/v1/chat/completions)</option>
              <option value="anthropic">Anthropic Native (/v1/messages)</option>
              <option value="gemini">Google Gemini Native</option>
              <option value="responses">OpenAI Realtime / Responses</option>
            </select>
          </div>

          <div class="sm:col-span-2">
            <label class="block text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-1">
              API Endpoint (Base URL) <span class="text-rose-500">*</span>
            </label>
            <input
              v-model="form.base_url"
              type="url"
              required
              placeholder="https://api.openlux.ai/v1"
              class="w-full rounded-xl border border-gray-200 bg-white px-3.5 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-700 dark:text-white"
            />
          </div>
        </div>

        <!-- Credentials (API Key) -->
        <div>
          <div class="flex items-center justify-between mb-1">
            <label class="block text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
              API Key (Credentials) <span v-if="!editingProvider" class="text-rose-500">*</span>
            </label>
            <span class="text-[11px] text-emerald-600 dark:text-emerald-400 flex items-center gap-1">
              <Icon name="check" size="xs" /> Encrypted with AES-256-GCM in DB
            </span>
          </div>
          <div class="relative">
            <input
              v-model="form.api_key"
              :type="showApiKey ? 'text' : 'password'"
              :required="!editingProvider"
              :placeholder="editingProvider ? '•••••••••••••••••••••••• (leave empty to keep current key)' : 'sk-...' "
              class="w-full rounded-xl border border-gray-200 bg-white py-2 pl-3.5 pr-10 text-sm font-mono text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-700 dark:text-white"
            />
            <button
              type="button"
              @click="showApiKey = !showApiKey"
              class="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
            >
              <Icon :name="showApiKey ? 'eyeOff' : 'eye'" size="sm" />
            </button>
          </div>
        </div>

        <!-- Balance, Min Balance & Status -->
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div>
            <label class="block text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-1">
              Initial / Current Balance ($ USD)
            </label>
            <div class="relative">
              <span class="absolute left-3.5 top-1/2 -translate-y-1/2 text-sm text-gray-400">$</span>
              <input
                v-model.number="form.balance_usd"
                type="number"
                step="0.01"
                min="0"
                placeholder="0.00"
                class="w-full rounded-xl border border-gray-200 bg-white py-2 pl-8 pr-3 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-700 dark:text-white"
              />
            </div>
            <p class="mt-1 text-[11px] text-gray-400">For manual sync / estimated balance tracking when balance API is absent.</p>
          </div>

          <div>
            <label class="block text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-1">
              Minimum Balance ($ USD)
            </label>
            <div class="relative">
              <span class="absolute left-3.5 top-1/2 -translate-y-1/2 text-sm text-gray-400">$</span>
              <input
                v-model.number="form.min_balance_usd"
                type="number"
                step="0.01"
                min="0"
                placeholder="20.00"
                class="w-full rounded-xl border border-gray-200 bg-white py-2 pl-8 pr-3 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-700 dark:text-white"
              />
            </div>
            <p class="mt-1 text-[11px] text-gray-400">Default $20. An alert is sent via Telegram when balance drops below this threshold.</p>
          </div>

          <div>
            <label class="block text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-1">
              Usage Status
            </label>
            <select
              v-model="form.status"
              class="w-full rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-700 dark:text-white"
            >
              <option value="active">Active (Enabled in Hypervisor scheduler)</option>
              <option value="disabled">Disabled (Inactive)</option>
            </select>
          </div>
        </div>

        <!-- Supported Model Families -->
        <div>
          <label class="block text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-2">
            Supported Model Families
          </label>
          <div class="flex flex-wrap gap-2.5">
            <label
              v-for="family in availableModelFamilies"
              :key="family.id"
              class="flex cursor-pointer items-center gap-2 rounded-xl border px-3 py-1.5 text-xs font-medium transition-all"
              :class="form.supported_model_families.includes(family.id)
                ? 'border-primary-500 bg-primary-50/70 text-primary-700 dark:border-primary-500/50 dark:bg-primary-950/40 dark:text-primary-300'
                : 'border-gray-200 bg-white text-gray-600 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600'"
            >
              <input
                type="checkbox"
                :value="family.id"
                v-model="form.supported_model_families"
                class="rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
              />
              <span>{{ family.label }}</span>
            </label>
          </div>
        </div>

        <!-- Model Mapping Editor (canonical <-> upstream) -->
        <div>
          <div class="flex items-center justify-between mb-2">
            <div>
              <label class="block text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                Model Mappings (Canonical Model ↔ Upstream Model)
              </label>
              <p class="text-[11px] text-gray-400">Override system canonical model names to provider-specific upstream names</p>
            </div>
            <button
              type="button"
              @click="addMappingRow"
              class="inline-flex items-center gap-1 rounded-lg border border-gray-200 px-2.5 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
            >
              <Icon name="plus" size="xs" />
              <span>Add Row</span>
            </button>
          </div>

          <div v-if="mappingRows.length === 0" class="rounded-xl border border-dashed border-gray-200 p-3 text-center text-xs text-gray-400 dark:border-dark-700">
            No custom overrides (canonical names will be used 1-to-1)
          </div>
          <div v-else class="space-y-2 max-h-48 overflow-y-auto pr-1">
            <div
              v-for="(row, idx) in mappingRows"
              :key="idx"
              class="flex items-center gap-2"
            >
              <input
                v-model="row.canonical"
                type="text"
                placeholder="Canonical (e.g. claude-3-7-sonnet)"
                class="flex-1 rounded-xl border border-gray-200 bg-white px-3 py-1.5 text-xs text-gray-900 focus:border-primary-500 focus:outline-none dark:border-dark-600 dark:bg-dark-700 dark:text-white"
              />
              <span class="text-gray-400 font-bold">→</span>
              <input
                v-model="row.upstream"
                type="text"
                placeholder="Upstream (e.g. claude-3-7-sonnet-20250219)"
                class="flex-1 rounded-xl border border-gray-200 bg-white px-3 py-1.5 text-xs text-gray-900 focus:border-primary-500 focus:outline-none dark:border-dark-600 dark:bg-dark-700 dark:text-white"
              />
              <button
                type="button"
                @click="removeMappingRow(idx)"
                class="rounded-lg p-1.5 text-gray-400 hover:text-rose-500"
              >
                <Icon name="x" size="xs" />
              </button>
            </div>
          </div>
        </div>

        <!-- Connection Test Result Banner in Modal -->
        <div v-if="draftTestResult" class="rounded-xl p-3 text-xs" :class="draftTestResult.success ? 'bg-emerald-50 text-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-300' : 'bg-rose-50 text-rose-800 dark:bg-rose-950/40 dark:text-rose-300'">
          <div class="flex items-center gap-2 font-semibold">
            <Icon :name="draftTestResult.success ? 'check' : 'x'" size="xs" />
            <span>{{ draftTestResult.success ? 'Connection successful!' : 'Connection error' }}</span>
          </div>
          <p class="mt-1">
            {{ draftTestResult.success
              ? `Latency: ${draftTestResult.latency_ms} ms | Available models: ${draftTestResult.model_count}`
              : (draftTestResult.error || 'Please check Base URL and API Key')
            }}
          </p>
        </div>

        <!-- Modal Actions -->
        <div class="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-dark-700">
          <button
            type="button"
            @click="testDraftCredentials"
            :disabled="isTestingDraft || (!editingProvider && !form.api_key)"
            class="inline-flex items-center gap-1.5 rounded-xl border border-gray-200 px-3.5 py-2 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
          >
            <Icon name="play" size="xs" :class="{ 'animate-spin': isTestingDraft }" />
            <span>{{ isTestingDraft ? 'Testing...' : 'Test Connection' }}</span>
          </button>

          <div class="flex items-center gap-2.5">
            <button
              type="button"
              @click="closeEditModal"
              class="rounded-xl border border-gray-200 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
            >
              Cancel
            </button>
            <button
              type="submit"
              :disabled="isSaving"
              class="inline-flex items-center gap-2 rounded-xl bg-primary-600 px-5 py-2 text-sm font-medium text-white shadow-sm hover:bg-primary-500 disabled:opacity-50 dark:bg-primary-500 dark:hover:bg-primary-400"
            >
              <Icon v-if="isSaving" name="sync" size="xs" class="animate-spin" />
              <span>{{ isSaving ? 'Saving...' : (editingProvider ? 'Save Changes' : 'Create Provider') }}</span>
            </button>
          </div>
        </div>
      </form>
    </BaseDialog>

    <!-- Mappings View Modal -->
    <BaseDialog
      :show="showMappingsModal"
      :title="'Model Mappings: ' + (activeMappingsProvider?.provider_name || '')"
      width="normal"
      @close="showMappingsModal = false"
    >
      <div v-if="activeMappingsProvider" class="space-y-4">
        <div class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
          <table class="w-full text-left text-xs">
            <thead class="bg-gray-50 dark:bg-dark-800 text-gray-500 uppercase font-semibold">
              <tr>
                <th class="px-4 py-2.5">Canonical Model</th>
                <th class="px-4 py-2.5">Upstream Model</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="(upstream, canonical) in activeMappingsProvider.model_mapping" :key="canonical">
                <td class="px-4 py-2.5 font-mono text-gray-900 dark:text-white font-medium">{{ canonical }}</td>
                <td class="px-4 py-2.5 font-mono text-primary-600 dark:text-primary-400">{{ upstream }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </BaseDialog>

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showDeleteConfirm"
      title="Delete Provider"
      :message="`Are you sure you want to delete provider «${deletingProvider?.provider_name}»? This action cannot be undone and will stop routing requests through this gateway.`"
      confirmText="Delete"
      cancelText="Cancel"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteConfirm = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import {
  getProviders,
  createProvider,
  updateProvider,
  deleteProvider,
  testProviderConnection,
  testDraftProviderConnection,
  syncProviderBalance,
  syncAllProviderBalances,
  type Provider,
  type ProviderState,
  type ProviderProtocol,
  type ProviderSummaryStats,
  type TestConnectionResponse
} from '@/api/admin/providers'

const appStore = useAppStore()

// State
const providers = ref<Provider[]>([])
const isLoading = ref(false)
const isSaving = ref(false)
const isSyncingAll = ref(false)
const syncingProviderId = ref<number | null>(null)
const testingProviderId = ref<number | null>(null)
const isTestingDraft = ref(false)
const draftTestResult = ref<TestConnectionResponse | null>(null)

// Pagination
const currentPage = ref(1)
const pageSize = ref(20)
const totalProviders = ref(0)
const totalPages = computed(() => Math.ceil(totalProviders.value / pageSize.value) || 1)

// Summary Stats
const summaryStats = ref<ProviderSummaryStats>({
  total_providers: 0,
  active_providers: 0,
  low_balance_providers: 0,
  total_balance_usd: 0,
  total_spend_usd: 0
})

// Filters
const filters = reactive({
  search: '',
  status: '',
  state: ''
})

let searchTimeout: any = null
function handleSearchInput() {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    currentPage.value = 1
    fetchData()
  }, 350)
}

function resetFilters() {
  filters.search = ''
  filters.status = ''
  filters.state = ''
  currentPage.value = 1
  fetchData()
}

// Model Families
const availableModelFamilies = [
  { id: 'claude', label: 'Claude (Anthropic)' },
  { id: 'gpt', label: 'GPT / Codex (OpenAI)' },
  { id: 'gemini', label: 'Gemini (Google)' },
  { id: 'kimi', label: 'Kimi (Moonshot)' },
  { id: 'deepseek', label: 'DeepSeek' },
  { id: 'other', label: 'Other / Custom' }
]

// Presets
const presets = [
  {
    id: 'openlux',
    name: 'OpenLux',
    domain: 'api.openlux.ai',
    base_url: 'https://api.openlux.ai/v1',
    protocol: 'openai' as ProviderProtocol,
    families: ['claude', 'gpt']
  },
  {
    id: 'portdan',
    name: 'Portdan',
    domain: 'api.portdan.com',
    base_url: 'https://api.portdan.com/v1',
    protocol: 'openai' as ProviderProtocol,
    families: ['claude', 'gpt', 'gemini']
  },
  {
    id: 'yuanqiaoyun',
    name: 'Yuanqiaoyun',
    domain: 'api.yuanqiaoyun.com',
    base_url: 'https://api.yuanqiaoyun.com/v1',
    protocol: 'openai' as ProviderProtocol,
    families: ['claude', 'gpt', 'kimi']
  },
  {
    id: 'api-top',
    name: 'API-Top',
    domain: 'api.api-top.net',
    base_url: 'https://api.api-top.net/v1',
    protocol: 'openai' as ProviderProtocol,
    families: ['claude', 'gpt']
  },
  {
    id: 'x-llm',
    name: 'X-LLM',
    domain: 'api.x-llm.com',
    base_url: 'https://api.x-llm.com/v1',
    protocol: 'openai' as ProviderProtocol,
    families: ['claude', 'gpt', 'deepseek']
  }
]
const selectedPresetId = ref<string | null>(null)

// Form
const showEditModal = ref(false)
const editingProvider = ref<Provider | null>(null)
const showApiKey = ref(false)

interface MappingRow {
  canonical: string
  upstream: string
}
const mappingRows = ref<MappingRow[]>([])

const form = reactive({
  provider_name: '',
  domain: '',
  protocol: 'openai' as ProviderProtocol,
  base_url: '',
  api_key: '',
  balance_usd: 0,
  min_balance_usd: 20,
  status: 'active' as 'active' | 'disabled',
  supported_model_families: [] as string[]
})

// Mappings Modal
const showMappingsModal = ref(false)
const activeMappingsProvider = ref<Provider | null>(null)

// Delete Dialog
const showDeleteConfirm = ref(false)
const deletingProvider = ref<Provider | null>(null)

// Fetch Data
async function fetchData() {
  isLoading.value = true
  try {
    const res = await getProviders(currentPage.value, pageSize.value, {
      search: filters.search || undefined,
      status: filters.status || undefined,
      state: filters.state || undefined
    })
    providers.value = (res.items && res.items.length > 0 ? res.items : res.providers) || []
    totalProviders.value = res.total || 0
    if (res.summary) {
      summaryStats.value = res.summary
    }
  } catch (err: any) {
    appStore.showToast('error', err?.message || 'Failed to load providers')
  } finally {
    isLoading.value = false
  }
}

function handlePageChange(page: number) {
  currentPage.value = page
  fetchData()
}

// Preset Handler
function applyPreset(preset: typeof presets[0]) {
  selectedPresetId.value = preset.id
  form.provider_name = preset.name
  form.domain = preset.domain
  form.base_url = preset.base_url
  form.protocol = preset.protocol
  form.supported_model_families = [...preset.families]
}

// Create & Edit Modal

function openCreateModal() {
  editingProvider.value = null
  selectedPresetId.value = null
  showApiKey.value = false
  draftTestResult.value = null
  form.provider_name = ''
  form.domain = ''
  form.protocol = 'openai'
  form.base_url = ''
  form.api_key = ''
  form.balance_usd = 0
  form.min_balance_usd = 20
  form.status = 'active'
  form.supported_model_families = ['claude', 'gpt']
  mappingRows.value = []
  showEditModal.value = true
}

function openEditModal(provider: Provider) {
  editingProvider.value = provider
  selectedPresetId.value = null
  showApiKey.value = false
  draftTestResult.value = null
  form.provider_name = provider.provider_name
  form.domain = provider.domain
  form.protocol = provider.protocol || 'openai'
  form.base_url = provider.base_url
  form.api_key = ''
  form.balance_usd = provider.balance_usd !== undefined ? provider.balance_usd : 0
  form.min_balance_usd = provider.min_balance_usd !== undefined ? provider.min_balance_usd : 20
  form.status = provider.status || 'active'
  form.supported_model_families = [...(provider.supported_model_families || [])]

  // Parse mappings
  mappingRows.value = Object.entries(provider.model_mapping || {}).map(([c, u]) => ({
    canonical: c,
    upstream: u
  }))

  showEditModal.value = true
}

function closeEditModal() {
  showEditModal.value = false
  editingProvider.value = null
  draftTestResult.value = null
}

function addMappingRow() {
  mappingRows.value.push({ canonical: '', upstream: '' })
}

function removeMappingRow(index: number) {
  mappingRows.value.splice(index, 1)
}

function buildModelMapping(): Record<string, string> {
  const result: Record<string, string> = {}
  for (const row of mappingRows.value) {
    const c = row.canonical.trim()
    const u = row.upstream.trim()
    if (c && u) {
      result[c] = u
    }
  }
  return result
}

// Save Provider
async function saveProvider() {
  isSaving.value = true
  try {
    const mapping = buildModelMapping()
    if (editingProvider.value) {
      await updateProvider(editingProvider.value.id, {
        provider_name: form.provider_name,
        domain: form.domain || undefined,
        protocol: form.protocol,
        base_url: form.base_url,
        api_key: form.api_key || undefined,
        balance_usd: form.balance_usd,
        min_balance_usd: form.min_balance_usd,
        status: form.status,
        supported_model_families: form.supported_model_families,
        model_mapping: mapping
      })
      appStore.showToast('success', `Provider «${form.provider_name}» updated successfully`)
    } else {
      await createProvider({
        provider_name: form.provider_name,
        domain: form.domain || undefined,
        protocol: form.protocol,
        base_url: form.base_url,
        api_key: form.api_key,
        balance_usd: form.balance_usd,
        min_balance_usd: form.min_balance_usd,
        status: form.status,
        supported_model_families: form.supported_model_families,
        model_mapping: mapping
      })
      appStore.showToast('success', `Provider «${form.provider_name}» created successfully`)
    }
    closeEditModal()
    fetchData()
  } catch (err: any) {
    appStore.showToast('error', err?.message || 'Failed to save provider')
  } finally {
    isSaving.value = false
  }
}

// Test Connection for Existing Provider
async function handleTestConnection(provider: Provider) {
  testingProviderId.value = provider.id
  try {
    const res = await testProviderConnection(provider.id)
    if (res.success) {
      appStore.showToast('success', `Connection to «${provider.provider_name}» verified (Latency: ${res.latency_ms} ms, Models: ${res.model_count})`)
    } else {
      appStore.showToast('error', `Connection test failed: ${res.error || 'Status ' + res.status_code}`)
    }
    fetchData()
  } catch (err: any) {
    appStore.showToast('error', err?.message || 'Connection test error')
  } finally {
    testingProviderId.value = null
  }
}

// Test Draft Credentials in Modal
async function testDraftCredentials() {
  isTestingDraft.value = true
  draftTestResult.value = null
  try {
    if (editingProvider.value && !form.api_key) {
      const res = await testProviderConnection(editingProvider.value.id)
      draftTestResult.value = res
    } else {
      const res = await testDraftProviderConnection({
        base_url: form.base_url,
        protocol: form.protocol,
        api_key: form.api_key
      })
      draftTestResult.value = res
    }
  } catch (err: any) {
    draftTestResult.value = {
      success: false,
      latency_ms: 0,
      status_code: 500,
      model_count: 0,
      error: err?.message || 'Connection error'
    }
  } finally {
    isTestingDraft.value = false
  }
}

// Sync Balance for Single Provider
async function handleSyncBalance(provider: Provider) {
  syncingProviderId.value = provider.id
  try {
    const res = await syncProviderBalance(provider.id)
    appStore.showToast('success', `Balance for «${provider.provider_name}» synced: $${formatCurrency(res.balance_usd)}`)
    fetchData()
  } catch (err: any) {
    appStore.showToast('error', err?.message || 'Failed to sync balance')
  } finally {
    syncingProviderId.value = null
  }
}

// Sync All Balances
async function handleSyncAll() {
  isSyncingAll.value = true
  try {
    const summary = await syncAllProviderBalances()
    if (summary.failed === 0) {
      appStore.showToast('success', `All balances synced successfully (${summary.successful}/${summary.total})`)
    } else {
      appStore.showToast('warning', `Synced: ${summary.successful}, failed: ${summary.failed}`)
    }
    fetchData()
  } catch (err: any) {
    appStore.showToast('error', err?.message || 'Batch balance sync failed')
  } finally {
    isSyncingAll.value = false
  }
}

// Toggle Provider Status
async function toggleProviderStatus(provider: Provider) {
  const newStatus = provider.status === 'active' ? 'disabled' : 'active'
  try {
    await updateProvider(provider.id, { status: newStatus })
    provider.status = newStatus
    appStore.showToast('success', `Provider «${provider.provider_name}» ${newStatus === 'active' ? 'enabled' : 'disabled'}`)
    fetchData()
  } catch (err: any) {
    appStore.showToast('error', err?.message || 'Failed to update status')
  }
}

// Mappings Modal
function openMappingsModal(provider: Provider) {
  activeMappingsProvider.value = provider
  showMappingsModal.value = true
}

// Delete Handlers
function openDeleteDialog(provider: Provider) {
  deletingProvider.value = provider
  showDeleteConfirm.value = true
}

async function confirmDelete() {
  if (!deletingProvider.value) return
  try {
    await deleteProvider(deletingProvider.value.id)
    appStore.showToast('success', `Provider «${deletingProvider.value.provider_name}» deleted`)
    showDeleteConfirm.value = false
    deletingProvider.value = null
    fetchData()
  } catch (err: any) {
    appStore.showToast('error', err?.message || 'Failed to delete provider')
  }
}

// Formatting & Styling Helpers
function formatCurrency(val?: number): string {
  if (val === undefined || val === null || isNaN(val)) return '0.00'
  return Number(val).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

function formatCompactNumber(num?: number): string {
  if (!num) return '0'
  if (num >= 1_000_000) return (num / 1_000_000).toFixed(1) + 'M'
  if (num >= 1_000) return (num / 1_000).toFixed(1) + 'K'
  return num.toString()
}

function formatTimeAgo(timeStr?: string | null): string {
  if (!timeStr) return 'Never synced'
  const diff = Date.now() - new Date(timeStr).getTime()
  const sec = Math.floor(diff / 1000)
  if (sec < 60) return 'just now'
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min}m ago`
  const hrs = Math.floor(min / 60)
  if (hrs < 24) return `${hrs}h ago`
  const days = Math.floor(hrs / 24)
  return `${days}d ago`
}

function getStateBadgeClass(state: ProviderState): string {
  switch (state) {
    case 'Healthy':
      return 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
    case 'Degraded':
      return 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20'
    case 'Down':
      return 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20'
    case 'Low balance':
      return 'bg-rose-500/15 text-rose-600 dark:text-rose-400 border border-rose-500/30 animate-pulse'
    case 'Invalid key':
      return 'bg-orange-500/10 text-orange-600 dark:text-orange-400 border border-orange-500/20'
    case 'Disabled':
    default:
      return 'bg-slate-500/10 text-slate-500 border border-slate-500/20'
  }
}

function getStateDotClass(state: ProviderState): string {
  switch (state) {
    case 'Healthy':
      return 'bg-emerald-500'
    case 'Degraded':
      return 'bg-amber-500'
    case 'Down':
      return 'bg-rose-500'
    case 'Low balance':
      return 'bg-rose-500'
    case 'Invalid key':
      return 'bg-orange-500'
    case 'Disabled':
    default:
      return 'bg-slate-400'
  }
}

function getProtocolBadgeClass(proto: ProviderProtocol): string {
  switch (proto) {
    case 'openai':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-400 border border-emerald-200 dark:border-emerald-800'
    case 'anthropic':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-400 border border-amber-200 dark:border-amber-800'
    case 'gemini':
      return 'bg-blue-50 text-blue-700 dark:bg-blue-950/40 dark:text-blue-400 border border-blue-200 dark:border-blue-800'
    case 'responses':
      return 'bg-purple-50 text-purple-700 dark:bg-purple-950/40 dark:text-purple-400 border border-purple-200 dark:border-purple-800'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  }
}

function getLatencyClass(latency?: number): string {
  if (!latency || latency <= 0) return 'text-gray-400'
  if (latency < 250) return 'text-emerald-600 dark:text-emerald-400'
  if (latency < 800) return 'text-amber-600 dark:text-amber-400'
  return 'text-rose-600 dark:text-rose-400'
}

function getErrorRateClass(rate?: number): string {
  if (rate === undefined || rate <= 0) return 'text-emerald-600 dark:text-emerald-400'
  if (rate < 2.0) return 'text-amber-600 dark:text-amber-400'
  return 'text-rose-600 dark:text-rose-400'
}

function getProviderAvatarColor(name: string): string {
  const colors = [
    '#3b82f6', '#8b5cf6', '#ec4899', '#f97316', '#10b981', '#06b6d4', '#6366f1'
  ]
  let hash = 0
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash)
  }
  return colors[Math.abs(hash) % colors.length]
}

onMounted(() => {
  fetchData()
})
</script>
