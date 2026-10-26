<template>
  <AppLayout>
    <TablePageLayout page-scroll>
      <template #actions>
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <!-- Total Requests -->
          <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30">
              <Icon name="document" size="md" class="text-blue-600 dark:text-blue-400" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('usage.totalRequests') }}
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                {{ usageStats?.total_requests?.toLocaleString() || '0' }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('usage.inSelectedRange') }}
              </p>
            </div>
          </div>
        </div>

        <!-- Total Tokens -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-amber-100 p-2 dark:bg-amber-900/30">
              <Icon name="cube" size="md" class="text-amber-600 dark:text-amber-400" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('usage.totalTokens') }}
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                {{ formatTokens(usageStats?.total_tokens || 0) }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('usage.in') }}: {{ formatTokens(usageStats?.total_input_tokens || 0) }} /
                {{ t('usage.out') }}: {{ formatTokens(usageStats?.total_output_tokens || 0) }}
              </p>
            </div>
          </div>
        </div>

        <!-- Total Cost -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-green-100 p-2 dark:bg-green-900/30">
              <Icon name="dollar" size="md" class="text-green-600 dark:text-green-400" />
            </div>
            <div class="min-w-0 flex-1">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('usage.totalCost') }}
              </p>
              <p class="text-xl font-bold text-green-600 dark:text-green-400">
                ${{ (usageStats?.total_actual_cost || 0).toFixed(4) }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('usage.actualCost') }} /
                <span class="line-through">${{ (usageStats?.total_cost || 0).toFixed(4) }}</span>
                {{ t('usage.standardCost') }}
              </p>
            </div>
          </div>
        </div>

        <!-- Average Duration -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-purple-100 p-2 dark:bg-purple-900/30">
              <Icon name="clock" size="md" class="text-purple-600 dark:text-purple-400" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('usage.avgDuration') }}
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                {{ formatDuration(usageStats?.average_duration_ms || 0) }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.perRequest') }}</p>
            </div>
          </div>
        </div>

        <!-- Live RPM (last 5 minutes) -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-cyan-100 p-2 dark:bg-cyan-900/30">
              <Icon name="bolt" size="md" class="text-cyan-600 dark:text-cyan-400" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('usage.rpm') }}</p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">{{ rpm.toFixed(1) }}</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.liveHint') }}</p>
            </div>
          </div>
        </div>

        <!-- Live TPM (last 5 minutes) -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-indigo-100 p-2 dark:bg-indigo-900/30">
              <Icon name="cube" size="md" class="text-indigo-600 dark:text-indigo-400" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('usage.tpm') }}</p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">{{ formatTokens(tpm) }}</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.liveHint') }}</p>
            </div>
          </div>
        </div>

        <!-- Used quota (today) -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-teal-100 p-2 dark:bg-teal-900/30">
              <Icon name="fire" size="md" class="text-teal-600 dark:text-teal-400" />
            </div>
            <div class="min-w-0 flex-1">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('usage.usedQuota') }}</p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">${{ usedQuotaToday.toFixed(4) }}</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.usedQuotaHint') }}</p>
            </div>
          </div>
        </div>

        <!-- Estimated daily consumption -->
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-rose-100 p-2 dark:bg-rose-900/30">
              <Icon name="trendingUp" size="md" class="text-rose-600 dark:text-rose-400" />
            </div>
            <div class="min-w-0 flex-1">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('usage.estimatedDaily') }}</p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">${{ estimatedDailyCost.toFixed(4) }}</p>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.estimatedDailyHint') }}</p>
            </div>
          </div>
        </div>
        </div>
      </template>

      <template #filters>
        <div class="card">
          <div class="px-6 py-4">
          <div class="flex flex-wrap items-end gap-4">
            <!-- API Key Filter -->
            <div class="min-w-[180px]">
              <label class="input-label">{{ t('usage.apiKeyFilter') }}</label>
              <Select
                v-model="filters.api_key_id"
                :options="apiKeyOptions"
                :placeholder="t('usage.allApiKeys')"
                @change="applyFilters"
              />
            </div>

            <!-- Date Range Filter -->
            <div>
              <label class="input-label">{{ t('usage.timeRange') }}</label>
              <DateRangePicker
                v-model:start-date="startDate"
                v-model:end-date="endDate"
                @change="onDateRangeChange"
              />
            </div>

            <!-- Model Filter -->
            <div class="min-w-[160px]">
              <label class="input-label">{{ t('usage.filterModel') }}</label>
              <input
                v-model="modelFilter"
                type="text"
                class="input"
                :placeholder="t('usage.filterAllModels')"
                @keyup.enter="applyFilters"
              />
            </div>

            <!-- Group Filter -->
            <div class="min-w-[160px]">
              <label class="input-label">{{ t('usage.filterGroup') }}</label>
              <Select
                v-model="groupFilter"
                :options="groupOptions"
                :placeholder="t('usage.filterAllGroups')"
                @change="applyFilters"
              />
            </div>

            <!-- Request Type Filter (stream / sync) -->
            <div class="min-w-[140px]">
              <label class="input-label">{{ t('usage.filterType') }}</label>
              <Select
                v-model="streamFilter"
                :options="requestTypeOptions"
                :placeholder="t('usage.filterAllTypes')"
                @change="applyFilters"
              />
            </div>

            <!-- Actions -->
            <div class="ml-auto flex items-center gap-3">
              <button @click="applyFilters" :disabled="loading" class="btn btn-secondary">
                {{ t('common.refresh') }}
              </button>
              <button @click="resetFilters" class="btn btn-secondary">
                {{ t('common.reset') }}
              </button>
              <button @click="exportToCSV" :disabled="exporting" class="btn btn-primary">
                <svg
                  v-if="exporting"
                  class="-ml-1 mr-2 h-4 w-4 animate-spin"
                  fill="none"
                  viewBox="0 0 24 24"
                >
                  <circle
                    class="opacity-25"
                    cx="12"
                    cy="12"
                    r="10"
                    stroke="currentColor"
                    stroke-width="4"
                  ></circle>
                  <path
                    class="opacity-75"
                    fill="currentColor"
                    d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                  ></path>
                </svg>
                {{ exporting ? t('usage.exporting') : t('usage.exportCsv') }}
              </button>
            </div>
          </div>
        </div>
        </div>
      </template>

      <template #table>
        <div class="mb-3 flex items-center gap-2">
          <button
            v-for="tab in usageTabs"
            :key="tab.key"
            class="rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
            :class="
              activeUsageTab === tab.key
                ? 'bg-primary-600 text-white'
                : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600'
            "
            @click="switchUsageTab(tab.key)"
          >
            {{ tab.label }}
          </button>
        </div>

        <DataTable v-if="activeUsageTab === 'requests'" :columns="columns" :data="usageLogs" :loading="loading" :scrollable="false">
          <template #cell-model="{ row }">
            <div class="flex min-w-0 flex-col leading-5">
              <span class="min-w-0 truncate font-medium text-gray-900 dark:text-white" :title="row.model">
                {{ row.model }}
              </span>
              <span class="truncate text-xs text-gray-500 dark:text-dark-400" :title="row.api_key?.name">
                {{ row.api_key?.name || '—' }}
              </span>
            </div>
          </template>

          <template #cell-request_id="{ row }">
            <div class="flex min-w-0 flex-col leading-5">
              <button
                v-if="row.request_id"
                class="group inline-flex max-w-full items-center gap-1 truncate font-mono text-xs text-gray-500 hover:text-primary-600 dark:text-gray-400"
                :title="t('usage.copyRequestId')"
                @click="copyRequestId(row.request_id)"
              >
                <span class="max-w-[160px] truncate">{{ row.request_id }}</span>
                <span class="flex-shrink-0 opacity-0 transition-opacity group-hover:opacity-100">
                  {{ copiedRequestId === row.request_id ? '✓' : '⧉' }}
                </span>
              </button>
              <span v-else class="text-gray-400">—</span>
              <span
                class="block max-w-[180px] truncate text-xs text-gray-400 dark:text-dark-500"
                :title="formatUsageEndpoints(row)"
              >
                {{ formatUsageEndpoints(row) }}
              </span>
            </div>
          </template>

          <template #cell-stream="{ row }">
            <div class="flex min-w-0 flex-col items-start gap-1 leading-5">
              <span
                class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium"
                :class="getRequestTypeBadgeClass(row)"
              >
                {{ getRequestTypeLabel(row) }}
              </span>
              <span
                v-if="row.user_agent"
                class="max-w-full truncate text-xs text-gray-400 dark:text-dark-500"
                :title="row.user_agent"
              >
                {{ formatUserAgent(row.user_agent) }}
              </span>
            </div>
          </template>

          <template #cell-tokens="{ row }">
            <!-- 图片生成请求 -->
            <div v-if="row.image_count > 0" class="flex items-center gap-1.5">
              <svg
                class="h-4 w-4 text-indigo-500"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"
                />
              </svg>
              <span class="font-medium text-gray-900 dark:text-white">{{ row.image_count }}{{ $t('usage.imageUnit') }}</span>
              <span class="text-gray-400">({{ row.image_size || '2K' }})</span>
            </div>
            <!-- Token 请求 -->
            <div v-else class="flex items-center gap-1.5">
              <div class="space-y-1.5 text-sm">
                <!-- Input / Output Tokens -->
                <div class="flex items-center gap-2">
                  <!-- Input -->
                  <div class="inline-flex items-center gap-1">
                    <Icon name="arrowDown" size="sm" class="text-emerald-500" />
                    <span class="font-medium text-gray-900 dark:text-white">{{
                      row.input_tokens.toLocaleString()
                    }}</span>
                  </div>
                  <!-- Output -->
                  <div class="inline-flex items-center gap-1">
                    <Icon name="arrowUp" size="sm" class="text-violet-500" />
                    <span class="font-medium text-gray-900 dark:text-white">{{
                      row.output_tokens.toLocaleString()
                    }}</span>
                  </div>
                </div>
                <!-- Cache Tokens (Read + Write) -->
                <div
                  v-if="row.cache_read_tokens > 0 || row.cache_creation_tokens > 0"
                  class="flex items-center gap-2"
                >
                  <!-- Cache Read -->
                  <div v-if="row.cache_read_tokens > 0" class="inline-flex items-center gap-1">
                    <Icon name="inbox" size="sm" class="text-sky-500" />
                    <span class="font-medium text-sky-600 dark:text-sky-400">{{
                      formatCacheTokens(row.cache_read_tokens)
                    }}</span>
                  </div>
                  <!-- Cache Write -->
                  <div v-if="row.cache_creation_tokens > 0" class="inline-flex items-center gap-1">
                    <Icon name="edit" size="sm" class="text-amber-500" />
                    <span class="font-medium text-amber-600 dark:text-amber-400">{{
                      formatCacheTokens(row.cache_creation_tokens)
                    }}</span>
                    <span v-if="row.cache_creation_1h_tokens > 0" class="inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-orange-100 text-orange-600 ring-1 ring-inset ring-orange-200 dark:bg-orange-500/20 dark:text-orange-400 dark:ring-orange-500/30">1h</span>
                    <span v-if="row.cache_ttl_overridden" :title="t('usage.cacheTtlOverriddenHint')" class="inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-rose-100 text-rose-600 ring-1 ring-inset ring-rose-200 dark:bg-rose-500/20 dark:text-rose-400 dark:ring-rose-500/30 cursor-help">R</span>
                  </div>
                </div>
              </div>
              <!-- Token Detail Tooltip -->
              <div
                class="group relative"
                @mouseenter="showTokenTooltip($event, row)"
                @mouseleave="hideTokenTooltip"
              >
                <div
                  class="flex h-4 w-4 cursor-help items-center justify-center rounded-full bg-gray-100 transition-colors group-hover:bg-blue-100 dark:bg-gray-700 dark:group-hover:bg-blue-900/50"
                >
                  <Icon
                    name="infoCircle"
                    size="xs"
                    class="text-gray-400 group-hover:text-blue-500 dark:text-gray-500 dark:group-hover:text-blue-400"
                  />
                </div>
              </div>
            </div>
          </template>

          <template #cell-cost="{ row }">
            <div class="flex items-center gap-1.5 text-sm">
              <span class="font-medium text-green-600 dark:text-green-400">
                ${{ row.actual_cost.toFixed(6) }}
              </span>
              <!-- Cost Detail Tooltip -->
              <div
                class="group relative"
                @mouseenter="showTooltip($event, row)"
                @mouseleave="hideTooltip"
              >
                <div
                  class="flex h-4 w-4 cursor-help items-center justify-center rounded-full bg-gray-100 transition-colors group-hover:bg-blue-100 dark:bg-gray-700 dark:group-hover:bg-blue-900/50"
                >
                  <Icon
                    name="infoCircle"
                    size="xs"
                    class="text-gray-400 group-hover:text-blue-500 dark:text-gray-500 dark:group-hover:text-blue-400"
                  />
                </div>
              </div>
            </div>
          </template>

          <template #cell-duration="{ row }">
            <div class="flex flex-col leading-5">
              <span class="text-sm text-gray-600 dark:text-gray-400">{{
                formatDuration(row.duration_ms)
              }}</span>
              <span
                v-if="row.first_token_ms != null"
                class="text-xs text-gray-400 dark:text-dark-500"
              >
                {{ t('usage.firstToken') }}: {{ formatDuration(row.first_token_ms) }}
              </span>
            </div>
          </template>

          <template #cell-created_at="{ value }">
            <div class="flex flex-col leading-5">
              <span class="whitespace-nowrap text-sm text-gray-600 dark:text-gray-400">{{
                formatDatePart(value)
              }}</span>
              <span class="whitespace-nowrap text-xs text-gray-400 dark:text-dark-500">{{
                formatTimePart(value)
              }}</span>
            </div>
          </template>

          <template #empty>
            <EmptyState :message="t('usage.noRecords')" />
          </template>
        </DataTable>

        <DataTable
          v-else-if="activeUsageTab === 'errors'"
          :columns="errorColumns"
          :data="errorLogs"
          :loading="errorLoading"
          :scrollable="false"
        >
          <template #cell-created_at="{ value }">
            <div class="flex flex-col leading-5">
              <span class="whitespace-nowrap text-sm text-gray-600 dark:text-gray-400">{{
                formatDatePart(value)
              }}</span>
              <span class="whitespace-nowrap text-xs text-gray-400 dark:text-dark-500">{{
                formatTimePart(value)
              }}</span>
            </div>
          </template>

          <template #cell-model="{ value }">
            <span
              class="block max-w-[120px] truncate font-medium text-gray-900 dark:text-white"
              :title="value || undefined"
            >{{ value || '-' }}</span>
          </template>

          <template #cell-status_code="{ row }">
            <div class="flex flex-col leading-5">
              <span class="font-mono text-xs font-semibold" :class="errorStatusClass(row.status_code)">
                {{ row.status_code || '-' }}
              </span>
              <span
                class="text-xs"
                :class="row.is_retryable ? 'text-amber-600 dark:text-amber-400' : 'text-gray-400 dark:text-gray-500'"
              >
                {{ row.is_retryable ? t('usage.retryable') : t('usage.notRetryable') }}
                <template v-if="row.retry_count > 0"> · {{ row.retry_count }}</template>
              </span>
            </div>
          </template>

          <template #cell-message="{ row }">
            <div class="flex min-w-0 flex-col leading-5">
              <span class="block max-w-[300px] truncate text-xs text-gray-600 dark:text-gray-300" :title="row.message">
                {{ row.message || '-' }}
              </span>
              <span class="block max-w-[300px] truncate text-xs text-gray-400 dark:text-dark-500" :title="row.error_type">
                {{ row.error_type || '—' }}
              </span>
            </div>
          </template>

          <template #cell-endpoint="{ row }">
            <span
              class="block max-w-[140px] truncate text-xs text-gray-600 dark:text-gray-300"
              :title="row.endpoint || undefined"
            >{{ row.endpoint || '—' }}</span>
          </template>

          <template #cell-request_id="{ row }">
            <button
              v-if="row.request_id"
              class="group inline-flex max-w-full items-center gap-1 truncate font-mono text-xs text-gray-500 hover:text-primary-600 dark:text-gray-400"
              :title="t('usage.copyRequestId')"
              @click="copyRequestId(row.request_id)"
            >
              <span class="max-w-[160px] truncate">{{ row.request_id }}</span>
              <span class="flex-shrink-0 opacity-0 transition-opacity group-hover:opacity-100">
                {{ copiedRequestId === row.request_id ? '✓' : '⧉' }}
              </span>
            </button>
            <span v-else class="text-gray-400">—</span>
          </template>

          <template #empty>
            <div class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
              {{ t('usage.noErrors') }}
            </div>
          </template>
        </DataTable>

        <!-- Money operations: deposits / usage / refunds / adjustments. The
             request log and the money ledger are separate read-models, so the
             ledger lives in its own tab rather than being mixed into the
             paginated request rows. -->
        <DataTable
          v-else-if="activeUsageTab === 'operations'"
          :columns="ledgerColumns"
          :data="ledgerEntries"
          :loading="ledgerLoading"
          :scrollable="false"
        >
          <template #cell-created_at="{ value }">
            <span class="whitespace-nowrap text-sm text-gray-600 dark:text-gray-300">{{
              formatDateTime(value)
            }}</span>
          </template>

          <template #cell-type="{ value }">
            <span class="inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium" :class="operationClass(value)">
              {{ operationLabel(value) }}
            </span>
          </template>

          <template #cell-amount_usd="{ value }">
            <span
              class="font-medium"
              :class="value >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-900 dark:text-white'"
            >
              {{ value >= 0 ? '+' : '-' }}${{ Math.abs(value).toFixed(4) }}
            </span>
          </template>

          <template #cell-description="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-300">{{ value || '—' }}</span>
          </template>

          <template #cell-reference="{ value }">
            <span class="font-mono text-xs text-gray-500 dark:text-gray-400">{{ value || '—' }}</span>
          </template>

          <template #empty>
            <div class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
              {{ t('usage.noOperations') }}
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="activePagination.total > 0"
          :page="activePagination.page"
          :total="activePagination.total"
          :page-size="activePagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>
  </AppLayout>

  <!-- Token Tooltip Portal -->
  <Teleport to="body">
    <div
      v-if="tokenTooltipVisible"
      class="fixed z-[9999] pointer-events-none -translate-y-1/2"
      :style="{
        left: tokenTooltipPosition.x + 'px',
        top: tokenTooltipPosition.y + 'px'
      }"
    >
      <div
        class="whitespace-nowrap rounded-lg border border-gray-700 bg-gray-900 px-3 py-2.5 text-xs text-white shadow-xl dark:border-gray-600 dark:bg-gray-800"
      >
        <div class="space-y-1.5">
          <!-- Token Breakdown -->
          <div>
            <div class="text-xs font-semibold text-gray-300 mb-1">{{ t('usage.tokenDetails') }}</div>
            <div v-if="tokenTooltipData && tokenTooltipData.input_tokens > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.inputTokens') }}</span>
              <span class="font-medium text-white">{{ tokenTooltipData.input_tokens.toLocaleString() }}</span>
            </div>
            <div v-if="tokenTooltipData && tokenTooltipData.output_tokens > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.outputTokens') }}</span>
              <span class="font-medium text-white">{{ tokenTooltipData.output_tokens.toLocaleString() }}</span>
            </div>
            <div v-if="tokenTooltipData && tokenTooltipData.cache_creation_tokens > 0">
              <!-- 有 5m/1h 明细时，展开显示 -->
              <template v-if="tokenTooltipData.cache_creation_5m_tokens > 0 || tokenTooltipData.cache_creation_1h_tokens > 0">
                <div v-if="tokenTooltipData.cache_creation_5m_tokens > 0" class="flex items-center justify-between gap-4">
                  <span class="text-gray-400 flex items-center gap-1.5">
                    {{ t('admin.usage.cacheCreation5mTokens') }}
                    <span class="inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-amber-500/20 text-amber-400 ring-1 ring-inset ring-amber-500/30">5m</span>
                  </span>
                  <span class="font-medium text-white">{{ tokenTooltipData.cache_creation_5m_tokens.toLocaleString() }}</span>
                </div>
                <div v-if="tokenTooltipData.cache_creation_1h_tokens > 0" class="flex items-center justify-between gap-4">
                  <span class="text-gray-400 flex items-center gap-1.5">
                    {{ t('admin.usage.cacheCreation1hTokens') }}
                    <span class="inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-orange-500/20 text-orange-400 ring-1 ring-inset ring-orange-500/30">1h</span>
                  </span>
                  <span class="font-medium text-white">{{ tokenTooltipData.cache_creation_1h_tokens.toLocaleString() }}</span>
                </div>
              </template>
              <!-- 无明细时，只显示聚合值 -->
              <div v-else class="flex items-center justify-between gap-4">
                <span class="text-gray-400">{{ t('admin.usage.cacheCreationTokens') }}</span>
                <span class="font-medium text-white">{{ tokenTooltipData.cache_creation_tokens.toLocaleString() }}</span>
              </div>
            </div>
            <div v-if="tokenTooltipData && tokenTooltipData.cache_ttl_overridden" class="flex items-center justify-between gap-4">
              <span class="text-gray-400 flex items-center gap-1.5">
                {{ t('usage.cacheTtlOverriddenLabel') }}
                <span class="inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-rose-500/20 text-rose-400 ring-1 ring-inset ring-rose-500/30">R-{{ tokenTooltipData.cache_creation_1h_tokens > 0 ? '5m' : '1H' }}</span>
              </span>
              <span class="font-medium text-rose-400">{{ tokenTooltipData.cache_creation_1h_tokens > 0 ? t('usage.cacheTtlOverridden1h') : t('usage.cacheTtlOverridden5m') }}</span>
            </div>
            <div v-if="tokenTooltipData && tokenTooltipData.cache_read_tokens > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.cacheReadTokens') }}</span>
              <span class="font-medium text-white">{{ tokenTooltipData.cache_read_tokens.toLocaleString() }}</span>
            </div>
          </div>
          <!-- Total -->
          <div class="flex items-center justify-between gap-6 border-t border-gray-700 pt-1.5">
            <span class="text-gray-400">{{ t('usage.totalTokens') }}</span>
            <span class="font-semibold text-blue-400">{{ ((tokenTooltipData?.input_tokens || 0) + (tokenTooltipData?.output_tokens || 0) + (tokenTooltipData?.cache_creation_tokens || 0) + (tokenTooltipData?.cache_read_tokens || 0)).toLocaleString() }}</span>
          </div>
        </div>
        <!-- Tooltip Arrow (left side) -->
        <div
          class="absolute right-full top-1/2 h-0 w-0 -translate-y-1/2 border-b-[6px] border-r-[6px] border-t-[6px] border-b-transparent border-r-gray-900 border-t-transparent dark:border-r-gray-800"
        ></div>
      </div>
    </div>
  </Teleport>

  <!-- Tooltip Portal -->
  <Teleport to="body">
    <div
      v-if="tooltipVisible"
      class="fixed z-[9999] pointer-events-none -translate-y-1/2"
      :style="{
        left: tooltipPosition.x + 'px',
        top: tooltipPosition.y + 'px'
      }"
    >
      <div
        class="whitespace-nowrap rounded-lg border border-gray-700 bg-gray-900 px-3 py-2.5 text-xs text-white shadow-xl dark:border-gray-600 dark:bg-gray-800"
      >
        <div class="space-y-1.5">
          <!-- Cost Breakdown -->
          <div class="mb-2 border-b border-gray-700 pb-1.5">
            <div class="text-xs font-semibold text-gray-300 mb-1">{{ t('usage.costDetails') }}</div>
            <div v-if="tooltipData && tooltipData.input_cost > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.inputCost') }}</span>
              <span class="font-medium text-white">${{ tooltipData.input_cost.toFixed(6) }}</span>
            </div>
            <div v-if="tooltipData && tooltipData.output_cost > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.outputCost') }}</span>
              <span class="font-medium text-white">${{ tooltipData.output_cost.toFixed(6) }}</span>
            </div>
            <div v-if="tooltipData && tooltipData.input_tokens > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('usage.inputTokenPrice') }}</span>
              <span class="font-medium text-sky-300">{{ formatTokenPricePerMillion(tooltipData.input_cost, tooltipData.input_tokens) }} {{ t('usage.perMillionTokens') }}</span>
            </div>
            <div v-if="tooltipData && tooltipData.output_tokens > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('usage.outputTokenPrice') }}</span>
              <span class="font-medium text-violet-300">{{ formatTokenPricePerMillion(tooltipData.output_cost, tooltipData.output_tokens) }} {{ t('usage.perMillionTokens') }}</span>
            </div>
            <div v-if="tooltipData && tooltipData.cache_creation_cost > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.cacheCreationCost') }}</span>
              <span class="font-medium text-white">${{ tooltipData.cache_creation_cost.toFixed(6) }}</span>
            </div>
            <div v-if="tooltipData && tooltipData.cache_read_cost > 0" class="flex items-center justify-between gap-4">
              <span class="text-gray-400">{{ t('admin.usage.cacheReadCost') }}</span>
              <span class="font-medium text-white">${{ tooltipData.cache_read_cost.toFixed(6) }}</span>
            </div>
          </div>
          <!-- Rate and Summary -->
          <div class="flex items-center justify-between gap-6">
            <span class="text-gray-400">{{ t('usage.serviceTier') }}</span>
            <span class="font-semibold text-cyan-300">{{ getUsageServiceTierLabel(tooltipData?.service_tier, t) }}</span>
          </div>
          <div class="flex items-center justify-between gap-6">
            <span class="text-gray-400">{{ t('usage.rate') }}</span>
            <span class="font-semibold text-blue-400"
              >{{ (tooltipData?.rate_multiplier || 1).toFixed(2) }}x</span
            >
          </div>
          <div class="flex items-center justify-between gap-6">
            <span class="text-gray-400">{{ t('usage.original') }}</span>
            <span class="font-medium text-white">${{ tooltipData?.total_cost.toFixed(6) }}</span>
          </div>
          <div class="flex items-center justify-between gap-6 border-t border-gray-700 pt-1.5">
            <span class="text-gray-400">{{ t('usage.billed') }}</span>
            <span class="font-semibold text-green-400"
              >${{ tooltipData?.actual_cost.toFixed(6) }}</span
            >
          </div>
        </div>
        <!-- Tooltip Arrow (left side) -->
        <div
          class="absolute right-full top-1/2 h-0 w-0 -translate-y-1/2 border-b-[6px] border-r-[6px] border-t-[6px] border-b-transparent border-r-gray-900 border-t-transparent dark:border-r-gray-800"
        ></div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, reactive, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { usageAPI, keysAPI, userGroupsAPI } from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Icon from '@/components/icons/Icon.vue'
import type { UsageLog, ApiKey, UsageQueryParams, UsageStatsResponse, Group } from '@/types'
import type { UsageErrorItem, UserDashboardStats } from '@/api/usage'
import { getLedger, type LedgerEntry, type LedgerEntryType } from '@/api/billing'
import type { Column } from '@/components/common/types'
import { formatDateTime, formatReasoningEffort } from '@/utils/format'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { formatTokenPricePerMillion } from '@/utils/usagePricing'
import { getUsageServiceTierLabel } from '@/utils/usageServiceTier'
import { resolveUsageRequestType } from '@/utils/usageRequestType'

const { t } = useI18n()
const appStore = useAppStore()

let abortController: AbortController | null = null
// Refreshes the RPM/TPM + today's-spend gauges while the page is open.
let liveTimer: ReturnType<typeof setInterval> | null = null

// Tooltip state
const tooltipVisible = ref(false)
const tooltipPosition = ref({ x: 0, y: 0 })
const tooltipData = ref<UsageLog | null>(null)

// Token tooltip state
const tokenTooltipVisible = ref(false)
const tokenTooltipPosition = ref({ x: 0, y: 0 })
const tokenTooltipData = ref<UsageLog | null>(null)

// Usage stats from API
const usageStats = ref<UsageStatsResponse | null>(null)

const copiedRequestId = ref('')
async function copyRequestId(id: string) {
  try {
    await navigator.clipboard.writeText(id)
    copiedRequestId.value = id
    setTimeout(() => {
      if (copiedRequestId.value === id) copiedRequestId.value = ''
    }, 1500)
  } catch (e) {
    console.warn('Copy failed:', e)
  }
}

// Column widths: every column gets "what its content needs" plus an equal share
// of the leftover, so the shares add back up to exactly the table width and the
// requests table never scrolls horizontally (same recipe as the API-keys table).
// `table-layout` stays auto, so a column can still grow past its share when its
// content needs more room.
const REQUEST_COL_NATURAL = 970
const REQUEST_COL = {
  model: `w-[calc(200px_+_(100%_-_${REQUEST_COL_NATURAL}px)_/_7)]`,
  request_id: `w-[calc(190px_+_(100%_-_${REQUEST_COL_NATURAL}px)_/_7)]`,
  stream: `w-[calc(110px_+_(100%_-_${REQUEST_COL_NATURAL}px)_/_7)]`,
  tokens: `w-[calc(130px_+_(100%_-_${REQUEST_COL_NATURAL}px)_/_7)]`,
  cost: `w-[calc(110px_+_(100%_-_${REQUEST_COL_NATURAL}px)_/_7)]`,
  duration: `w-[calc(130px_+_(100%_-_${REQUEST_COL_NATURAL}px)_/_7)]`,
  created_at: `w-[calc(100px_+_(100%_-_${REQUEST_COL_NATURAL}px)_/_7)]`
}

// Same recipe for the Errors and Operations tabs of the same screen.
const ERROR_COL_NATURAL = 980
const ERROR_COL = {
  created_at: `w-[calc(100px_+_(100%_-_${ERROR_COL_NATURAL}px)_/_6)]`,
  model: `w-[calc(150px_+_(100%_-_${ERROR_COL_NATURAL}px)_/_6)]`,
  status_code: `w-[calc(110px_+_(100%_-_${ERROR_COL_NATURAL}px)_/_6)]`,
  message: `w-[calc(300px_+_(100%_-_${ERROR_COL_NATURAL}px)_/_6)]`,
  endpoint: `w-[calc(150px_+_(100%_-_${ERROR_COL_NATURAL}px)_/_6)]`,
  request_id: `w-[calc(170px_+_(100%_-_${ERROR_COL_NATURAL}px)_/_6)]`
}

const LEDGER_COL_NATURAL = 960
const LEDGER_COL = {
  created_at: `w-[calc(160px_+_(100%_-_${LEDGER_COL_NATURAL}px)_/_6)]`,
  type: `w-[calc(140px_+_(100%_-_${LEDGER_COL_NATURAL}px)_/_6)]`,
  amount_usd: `w-[calc(110px_+_(100%_-_${LEDGER_COL_NATURAL}px)_/_6)]`,
  status: `w-[calc(110px_+_(100%_-_${LEDGER_COL_NATURAL}px)_/_6)]`,
  description: `w-[calc(280px_+_(100%_-_${LEDGER_COL_NATURAL}px)_/_6)]`,
  reference: `w-[calc(160px_+_(100%_-_${LEDGER_COL_NATURAL}px)_/_6)]`
}

const columns = computed<Column[]>(() => [
  { key: 'model', label: t('usage.model'), sortable: true, class: REQUEST_COL.model },
  { key: 'request_id', label: t('usage.requestId'), sortable: false, class: REQUEST_COL.request_id },
  { key: 'stream', label: t('usage.type'), sortable: false, class: REQUEST_COL.stream },
  { key: 'tokens', label: t('usage.tokens'), sortable: false, class: REQUEST_COL.tokens },
  { key: 'cost', label: t('usage.cost'), sortable: false, class: REQUEST_COL.cost },
  { key: 'duration', label: t('usage.duration'), sortable: false, class: REQUEST_COL.duration },
  { key: 'created_at', label: t('usage.time'), sortable: true, class: REQUEST_COL.created_at }
])

// ---- Errors tab -----------------------------------------------------------
// The request log only lists completed calls; this tab surfaces the caller's
// FAILED requests (status, error type/message, retry info) from the same
// operations data the admin sees, sanitized to the caller's own rows.
type UsageTab = 'requests' | 'errors' | 'operations'
const activeUsageTab = ref<UsageTab>('requests')
const usageTabs = computed(() => [
  { key: 'requests' as UsageTab, label: t('usage.tabRequests') },
  { key: 'errors' as UsageTab, label: t('usage.tabErrors') },
  { key: 'operations' as UsageTab, label: t('usage.tabOperations') }
])

const errorLogs = ref<UsageErrorItem[]>([])
const errorLoading = ref(false)
const errorPagination = reactive({ page: 1, page_size: 20, total: 0, pages: 1 })
let errorAbortController: AbortController | null = null

const errorColumns = computed<Column[]>(() => [
  { key: 'created_at', label: t('usage.time'), sortable: false, class: ERROR_COL.created_at },
  { key: 'model', label: t('usage.model'), sortable: false, class: ERROR_COL.model },
  { key: 'status_code', label: t('usage.status'), sortable: false, class: ERROR_COL.status_code },
  { key: 'message', label: t('usage.errorMessage'), sortable: false, class: ERROR_COL.message },
  { key: 'endpoint', label: t('usage.endpoint'), sortable: false, class: ERROR_COL.endpoint },
  { key: 'request_id', label: t('usage.requestId'), sortable: false, class: ERROR_COL.request_id }
])

/** Tri-state wrappers: the Select component needs a concrete value for "all". */
const modelFilter = computed({
  get: () => filters.value.model ?? '',
  set: (value: string) => {
    filters.value.model = value.trim() === '' ? undefined : value
  }
})

const groupFilter = computed({
  get: () => (filters.value.group_id === undefined ? '' : String(filters.value.group_id)),
  set: (value: string) => {
    filters.value.group_id = value === '' ? undefined : Number(value)
  }
})

const ledgerColumns = computed<Column[]>(() => [
  { key: 'created_at', label: t('usage.time'), sortable: false, class: LEDGER_COL.created_at },
  { key: 'type', label: t('usage.operation'), sortable: false, class: LEDGER_COL.type },
  { key: 'amount_usd', label: t('usage.amount'), sortable: false, class: LEDGER_COL.amount_usd },
  { key: 'status', label: t('usage.status'), sortable: false, class: LEDGER_COL.status },
  { key: 'description', label: t('usage.ledgerDescription'), sortable: false, class: LEDGER_COL.description },
  { key: 'reference', label: t('usage.reference'), sortable: false, class: LEDGER_COL.reference }
])

const errorStatusClass = (code: number): string => {
  if (code >= 500) return 'text-red-600 dark:text-red-400'
  if (code === 429) return 'text-amber-600 dark:text-amber-400'
  if (code >= 400) return 'text-orange-600 dark:text-orange-400'
  return 'text-gray-600 dark:text-gray-300'
}

const activePagination = computed(() => {
  if (activeUsageTab.value === 'errors') return errorPagination
  if (activeUsageTab.value === 'operations') return ledgerPagination
  return pagination
})

const loadUsageErrors = async () => {
  errorAbortController?.abort()
  const controller = new AbortController()
  errorAbortController = controller
  errorLoading.value = true
  try {
    const response = await usageAPI.listErrors(
      {
        page: errorPagination.page,
        page_size: errorPagination.page_size,
        api_key_id: filters.value.api_key_id,
        start_date: filters.value.start_date,
        end_date: filters.value.end_date
      },
      { signal: controller.signal }
    )
    if (controller.signal.aborted) return
    errorLogs.value = response.items
    errorPagination.total = response.total
    errorPagination.pages = response.pages ?? 1
  } catch (error) {
    if (controller.signal.aborted) return
    const abortError = error as { name?: string; code?: string }
    if (abortError?.name === 'AbortError' || abortError?.code === 'ERR_CANCELED') return
    appStore.showError(t('usage.failedToLoad'))
  } finally {
    if (errorAbortController === controller) {
      errorLoading.value = false
    }
  }
}

const switchUsageTab = (tab: UsageTab) => {
  activeUsageTab.value = tab
  if (tab === 'errors') {
    void loadUsageErrors()
  }
  if (tab === 'operations') {
    void loadLedger()
  }
}

const usageLogs = ref<UsageLog[]>([])
const apiKeys = ref<ApiKey[]>([])
const loading = ref(false)
const exporting = ref(false)

const apiKeyOptions = computed(() => {
  return [
    { value: null, label: t('usage.allApiKeys') },
    ...apiKeys.value.map((key) => ({
      value: key.id,
      label: key.name
    }))
  ]
})

// Helper function to format date in local timezone
const formatLocalDate = (date: Date): string => {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

// Initialize date range immediately
const now = new Date()
const weekAgo = new Date(now)
weekAgo.setDate(weekAgo.getDate() - 6)

// Date range state
const startDate = ref(formatLocalDate(weekAgo))
const endDate = ref(formatLocalDate(now))

const filters = ref<UsageQueryParams>({
  api_key_id: undefined,
  group_id: undefined,
  model: undefined,
  stream: undefined,
  start_date: undefined,
  end_date: undefined
})

// Initialize filters with date range
filters.value.start_date = startDate.value
filters.value.end_date = endDate.value

// ---- Live counters + daily projection --------------------------------------
// `GET /usage/dashboard/stats` already computes RPM/TPM as the last-five-minute
// averages and carries today's spend, so the top row can show what the account
// is doing right now without a new endpoint. Refreshed on a timer so the RPM/TPM
// gauges stay live.
const dashboardStats = ref<UserDashboardStats | null>(null)
const rpm = computed(() => dashboardStats.value?.rpm ?? 0)
const tpm = computed(() => dashboardStats.value?.tpm ?? 0)
const usedQuotaToday = computed(() => dashboardStats.value?.today_actual_cost ?? 0)

/** Today's spend projected over a full day from the hours elapsed so far. */
const estimatedDailyCost = computed(() => {
  const spent = usedQuotaToday.value
  if (spent <= 0) return 0
  const now = new Date()
  const elapsedHours = now.getHours() + now.getMinutes() / 60
  if (elapsedHours <= 0.05) return spent
  return (spent / elapsedHours) * 24
})

const loadDashboardStats = async () => {
  try {
    dashboardStats.value = await usageAPI.getDashboardStats()
  } catch (error) {
    console.error('Failed to load dashboard stats:', error)
  }
}

// ---- Filter options --------------------------------------------------------
const userGroups = ref<Group[]>([])

const loadGroups = async () => {
  try {
    userGroups.value = await userGroupsAPI.getAvailable()
  } catch (error) {
    console.error('Failed to load groups:', error)
  }
}

const groupOptions = computed(() => [
  { value: '', label: t('usage.filterAllGroups') },
  ...userGroups.value.map((group) => ({ value: group.id, label: group.name }))
])

const requestTypeOptions = computed(() => [
  { value: '', label: t('usage.filterAllTypes') },
  { value: 'true', label: t('usage.stream') },
  { value: 'false', label: t('usage.sync') }
])

/** The `stream` filter is a tri-state: undefined = every request type. */
const streamFilter = computed({
  get: () => (filters.value.stream === undefined ? '' : String(filters.value.stream)),
  set: (value: string) => {
    filters.value.stream = value === '' ? undefined : value === 'true'
  }
})

// ---- Operations tab (money ledger) -----------------------------------------
// OpenLux's log screen shows consumption and top-ups in ONE list with an
// operation badge. Here the request log and the money ledger are different
// read-models — the ledger is `GET /payment/ledger`, already used by /billing —
// so the ledger gets its own tab rather than being merged into paginated
// request rows, which could not be paginated correctly.
const ledgerEntries = ref<LedgerEntry[]>([])
const ledgerLoading = ref(false)
const ledgerPagination = reactive({ page: 1, page_size: 20, total: 0, pages: 1 })

const loadLedger = async () => {
  ledgerLoading.value = true
  try {
    const response = await getLedger({ page: ledgerPagination.page, page_size: ledgerPagination.page_size })
    ledgerEntries.value = response.items
    ledgerPagination.total = response.total
    ledgerPagination.pages = Math.max(1, Math.ceil(response.total / ledgerPagination.page_size))
  } catch (error) {
    console.error('Failed to load the money ledger:', error)
  } finally {
    ledgerLoading.value = false
  }
}

const operationLabel = (type: LedgerEntryType): string =>
  ({
    deposit: t('usage.operationTopUp'),
    usage: t('usage.operationConsume'),
    refund: t('usage.operationRefund'),
    adjustment: t('usage.operationAdjustment')
  })[type] ?? type

const operationClass = (type: LedgerEntryType): string =>
  ({
    deposit: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
    usage: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300',
    refund: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
    adjustment: 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300'
  })[type] ?? 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'

// Handle date range change from DateRangePicker
const onDateRangeChange = (range: {
  startDate: string
  endDate: string
  preset: string | null
}) => {
  filters.value.start_date = range.startDate
  filters.value.end_date = range.endDate
  applyFilters()
}

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})

const formatDuration = (ms: number): string => {
  if (ms < 1000) return `${ms.toFixed(0)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

// The TIME column renders as two lines (date over time) so it stays narrow
// without clipping: same locale-aware formatter, split by explicit options.
const formatDatePart = (value: string): string =>
  formatDateTime(value, { year: 'numeric', month: '2-digit', day: '2-digit' })
const formatTimePart = (value: string): string =>
  formatDateTime(value, { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })

const formatUserAgent = (ua: string): string => {
  return ua
}

const getRequestTypeLabel = (log: UsageLog): string => {
  const requestType = resolveUsageRequestType(log)
  if (requestType === 'ws_v2') return t('usage.ws')
  if (requestType === 'stream') return t('usage.stream')
  if (requestType === 'sync') return t('usage.sync')
  return t('usage.unknown')
}

const getRequestTypeBadgeClass = (log: UsageLog): string => {
  const requestType = resolveUsageRequestType(log)
  if (requestType === 'ws_v2') return 'bg-violet-100 text-violet-800 dark:bg-violet-900 dark:text-violet-200'
  if (requestType === 'stream') return 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200'
  if (requestType === 'sync') return 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200'
  return 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200'
}

const getRequestTypeExportText = (log: UsageLog): string => {
  const requestType = resolveUsageRequestType(log)
  if (requestType === 'ws_v2') return 'WS'
  if (requestType === 'stream') return 'Stream'
  if (requestType === 'sync') return 'Sync'
  return 'Unknown'
}

const formatUsageEndpoints = (log: UsageLog): string => {
  const inbound = log.inbound_endpoint?.trim()
  return inbound || '-'
}

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)}B`
  } else if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)}M`
  } else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)}K`
  }
  return value.toLocaleString()
}

// Compact format for cache tokens in table cells
const formatCacheTokens = (value: number): string => {
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(1)}M`
  } else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(1)}K`
  }
  return value.toLocaleString()
}

const loadUsageLogs = async () => {
  if (abortController) {
    abortController.abort()
  }
  const currentAbortController = new AbortController()
  abortController = currentAbortController
  const { signal } = currentAbortController
  loading.value = true
  try {
    const params: UsageQueryParams = {
      page: pagination.page,
      page_size: pagination.page_size,
      ...filters.value
    }

    const response = await usageAPI.query(params, { signal })
    if (signal.aborted) {
      return
    }
    usageLogs.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
  } catch (error) {
    if (signal.aborted) {
      return
    }
    const abortError = error as { name?: string; code?: string }
    if (abortError?.name === 'AbortError' || abortError?.code === 'ERR_CANCELED') {
      return
    }
    appStore.showError(t('usage.failedToLoad'))
  } finally {
    if (abortController === currentAbortController) {
      loading.value = false
    }
  }
}

const loadApiKeys = async () => {
  try {
    const response = await keysAPI.list(1, 100)
    apiKeys.value = response.items
  } catch (error) {
    console.error('Failed to load API keys:', error)
  }
}

const loadUsageStats = async () => {
  try {
    const apiKeyId = filters.value.api_key_id ? Number(filters.value.api_key_id) : undefined
    const stats = await usageAPI.getStatsByDateRange(
      filters.value.start_date || startDate.value,
      filters.value.end_date || endDate.value,
      apiKeyId
    )
    usageStats.value = stats
  } catch (error) {
    console.error('Failed to load usage stats:', error)
  }
}

const applyFilters = () => {
  pagination.page = 1
  loadUsageLogs()
  loadUsageStats()
}

const resetFilters = () => {
  filters.value = {
    api_key_id: undefined,
    group_id: undefined,
    model: undefined,
    stream: undefined,
    start_date: undefined,
    end_date: undefined
  }
  // Reset date range to default (last 7 days)
  const now = new Date()
  const weekAgo = new Date(now)
  weekAgo.setDate(weekAgo.getDate() - 6)
  startDate.value = formatLocalDate(weekAgo)
  endDate.value = formatLocalDate(now)
  filters.value.start_date = startDate.value
  filters.value.end_date = endDate.value
  pagination.page = 1
  loadUsageLogs()
  loadUsageStats()
}

const handlePageChange = (page: number) => {
  if (activeUsageTab.value === 'errors') {
    errorPagination.page = page
    void loadUsageErrors()
    return
  }
  if (activeUsageTab.value === 'operations') {
    ledgerPagination.page = page
    void loadLedger()
    return
  }
  pagination.page = page
  loadUsageLogs()
}

const handlePageSizeChange = (pageSize: number) => {
  if (activeUsageTab.value === 'errors') {
    errorPagination.page_size = pageSize
    errorPagination.page = 1
    void loadUsageErrors()
    return
  }
  if (activeUsageTab.value === 'operations') {
    ledgerPagination.page_size = pageSize
    ledgerPagination.page = 1
    ledgerPagination.pages = Math.max(1, Math.ceil(ledgerPagination.total / pageSize))
    void loadLedger()
    return
  }
  pagination.page_size = pageSize
  pagination.page = 1
  loadUsageLogs()
}

/**
 * Escape CSV value to prevent injection and handle special characters
 */
const escapeCSVValue = (value: unknown): string => {
  if (value == null) return ''

  const str = String(value)
  const escaped = str.replace(/"/g, '""')

  // Prevent formula injection by prefixing dangerous characters with single quote
  if (/^[=+\-@\t\r]/.test(str)) {
    return `"\'${escaped}"`
  }

  // Escape values containing comma, quote, or newline
  if (/[,"\n\r]/.test(str)) {
    return `"${escaped}"`
  }

  return str
}

const exportToCSV = async () => {
  if (pagination.total === 0) {
    appStore.showWarning(t('usage.noDataToExport'))
    return
  }

  exporting.value = true
  appStore.showInfo(t('usage.preparingExport'))

  try {
    const allLogs: UsageLog[] = []
    const pageSize = 100 // Use a larger page size for export to reduce requests
    const totalRequests = Math.ceil(pagination.total / pageSize)

    for (let page = 1; page <= totalRequests; page++) {
      const params: UsageQueryParams = {
        page: page,
        page_size: pageSize,
        ...filters.value
      }
      const response = await usageAPI.query(params)
      allLogs.push(...response.items)
    }

    if (allLogs.length === 0) {
      appStore.showWarning(t('usage.noDataToExport'))
      return
    }

    const headers = [
      'Time',
      'API Key Name',
      'Model',
      'Reasoning Effort',
      'Inbound Endpoint',
      'Type',
      'Input Tokens',
      'Output Tokens',
      'Cache Read Tokens',
      'Cache Creation Tokens',
      'Rate Multiplier',
      'Billed Cost',
      'Original Cost',
      'First Token (ms)',
      'Duration (ms)'
    ]
    const rows = allLogs.map((log) =>
      [
        log.created_at,
        log.api_key?.name || '',
        log.model,
        formatReasoningEffort(log.reasoning_effort),
        log.inbound_endpoint || '',
        getRequestTypeExportText(log),
        log.input_tokens,
        log.output_tokens,
        log.cache_read_tokens,
        log.cache_creation_tokens,
        log.rate_multiplier,
        log.actual_cost.toFixed(8),
        log.total_cost.toFixed(8),
        log.first_token_ms ?? '',
        log.duration_ms
      ].map(escapeCSVValue)
    )

    const csvContent = [
      headers.map(escapeCSVValue).join(','),
      ...rows.map((row) => row.join(','))
    ].join('\n')

    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' })
    const url = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `usage_${filters.value.start_date}_to_${filters.value.end_date}.csv`
    link.click()
    window.URL.revokeObjectURL(url)

    appStore.showSuccess(t('usage.exportSuccess'))
  } catch (error) {
    appStore.showError(t('usage.exportFailed'))
    console.error('CSV Export failed:', error)
  } finally {
    exporting.value = false
  }
}

// Tooltip functions
const showTooltip = (event: MouseEvent, row: UsageLog) => {
  const target = event.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()

  tooltipData.value = row
  // Position to the right of the icon, vertically centered
  tooltipPosition.value.x = rect.right + 8
  tooltipPosition.value.y = rect.top + rect.height / 2
  tooltipVisible.value = true
}

const hideTooltip = () => {
  tooltipVisible.value = false
  tooltipData.value = null
}

// Token tooltip functions
const showTokenTooltip = (event: MouseEvent, row: UsageLog) => {
  const target = event.currentTarget as HTMLElement
  const rect = target.getBoundingClientRect()

  tokenTooltipData.value = row
  tokenTooltipPosition.value.x = rect.right + 8
  tokenTooltipPosition.value.y = rect.top + rect.height / 2
  tokenTooltipVisible.value = true
}

const hideTokenTooltip = () => {
  tokenTooltipVisible.value = false
  tokenTooltipData.value = null
}

onMounted(() => {
  loadApiKeys()
  loadUsageLogs()
  loadUsageStats()
  loadDashboardStats()
  loadGroups()
  loadLedger()
  // Keep the RPM/TPM gauges and today's spend live while the page is open.
  liveTimer = setInterval(loadDashboardStats, 60000)
})

onUnmounted(() => {
  if (liveTimer) clearInterval(liveTimer)
  abortController?.abort()
  errorAbortController?.abort()
})
</script>
