<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Header section -->
      <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div class="mb-2 flex items-center gap-3">
            <router-link
              to="/admin/providers"
              class="inline-flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 transition-colors"
            >
              <span>← Back to Providers</span>
            </router-link>
            <span class="text-gray-300 dark:text-dark-600">|</span>
            <router-link
              to="/admin/vendors"
              class="inline-flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 transition-colors"
            >
              <span>💡 Cheaper Alternatives</span>
            </router-link>
            <span class="text-gray-300 dark:text-dark-600">|</span>
            <router-link
              to="/admin/provider-health"
              class="inline-flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 transition-colors"
            >
              <span>{{ t('ui.parserStatsView.providerHealth') }}</span>
            </router-link>
          </div>
          <h1 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-white">
            {{ t('ui.parserStatsView.providerParserStatistics') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('ui.parserStatsView.crawlExecutionMetrics') }} <code class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-primary-600 dark:bg-dark-800 dark:text-primary-400">data/run_stats.json</code>.
          </p>
        </div>

        <div class="flex flex-wrap items-center gap-3">
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 shadow-sm transition-all hover:bg-gray-50 disabled:opacity-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200"
            :disabled="loading"
            @click="fetchStats"
          >
            <Icon name="sync" size="sm" :class="{ 'animate-spin': loading }" />
            <span>{{ loading ? 'Refreshing...' : 'Refresh' }}</span>
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 shadow-sm transition-all hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200"
            @click="showRawModal = true"
          >
            <Icon name="link" size="sm" />
            <span>{{ t('ui.parserStatsView.rawJSON') }}</span>
          </button>
        </div>
      </div>

      <!-- File Status Banner -->
      <div
        class="rounded-2xl border p-4 shadow-sm"
        :class="stats?.file_exists ? 'border-gray-200/80 bg-white/85 dark:border-dark-700/80 dark:bg-dark-800/85' : 'border-amber-200 bg-amber-50/70 dark:border-amber-900/50 dark:bg-amber-950/20'"
      >
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="flex items-center gap-3">
            <span
              class="flex h-3 w-3 relative"
              :title="stats?.file_exists ? 'File present on disk' : 'File missing'"
            >
              <span
                v-if="stats?.file_exists"
                class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"
              ></span>
              <span
                class="relative inline-flex rounded-full h-3 w-3"
                :class="stats?.file_exists ? 'bg-emerald-500' : 'bg-amber-500'"
              ></span>
            </span>
            <div class="text-sm">
              <span class="font-medium text-gray-900 dark:text-white">
                {{ stats?.file_exists ? t('ui.parserStatsView.sourceFile') : t('ui.parserStatsView.fileStatus') }}
              </span>
              <span
                class="ml-2 font-mono text-xs"
                :class="stats?.file_exists ? 'text-gray-600 dark:text-gray-300' : 'text-amber-700 dark:text-amber-400 font-semibold'"
              >
                {{ stats?.file_exists ? stats.source_path : t('ui.parserStatsView.notFoundInCandidatePaths') }}
              </span>
            </div>
          </div>

          <div class="flex flex-wrap items-center gap-4 text-xs text-gray-500 dark:text-gray-400">
            <div v-if="stats?.last_sync_at">
              <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('ui.parserStatsView.lastSync') }}</span>
              <span class="ml-1.5">{{ formatDate(stats.last_sync_at) }}</span>
              <span class="ml-1 text-gray-400">({{ timeAgo(stats.last_sync_at) }})</span>
            </div>
            <div v-if="stats?.file_mod_time">
              <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('ui.parserStatsView.fileUpdated') }}</span>
              <span class="ml-1.5">{{ formatDate(stats.file_mod_time) }}</span>
            </div>
          </div>
        </div>

        <div v-if="!stats?.file_exists" class="mt-3 border-t border-amber-200/80 pt-3 text-xs text-amber-900 dark:border-amber-900/40 dark:text-amber-200">
          <p class="font-medium">
            ⚠️ File <code class="font-mono">run_stats.json</code> {{ t('ui.parserStatsView.couldnotberead') }}
          </p>
          <div v-if="stats?.candidate_paths && stats.candidate_paths.length" class="mt-2 max-h-48 overflow-y-auto rounded-lg border border-amber-300/60 bg-amber-100/50 p-2 dark:border-amber-900/60 dark:bg-amber-950/40 font-mono text-[11px] space-y-1">
            <div
              v-for="path in stats.candidate_paths"
              :key="path"
              class="flex flex-col sm:flex-row sm:items-center justify-between gap-1 py-0.5 border-b border-amber-200/40 dark:border-amber-900/30 last:border-0"
            >
              <span class="text-gray-800 dark:text-gray-200 break-all">{{ path }}</span>
              <span class="text-amber-700 dark:text-amber-400 text-[10px] shrink-0 font-sans">
                {{ stats.checked_errors?.[path] || t('ui.parserStatsView.notFound') }}
              </span>
            </div>
          </div>
          <p class="mt-2 text-[11px] text-amber-800/90 dark:text-amber-300/80">
            💡 Tip: You can set an exact path using environment variable <code class="font-mono">RUN_STATS_PATH=/path/to/run_stats.json</code> {{ t('ui.parserStatsView.orensureuserpermissions') }} <code class="font-mono">/root/bestai_prices_parser/public/data/run_stats.json</code>.
          </p>
        </div>
      </div>

      <!-- Current Crawl In-Progress Banner -->
      <div
        v-if="stats?.current_run && stats.current_run.status === 'running'"
        class="rounded-2xl border border-blue-200 bg-blue-50/80 p-5 shadow-sm dark:border-blue-900/50 dark:bg-blue-950/30"
      >
        <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div class="flex items-center gap-3">
            <span class="flex h-3 w-3 relative">
              <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-blue-400 opacity-75"></span>
              <span class="relative inline-flex rounded-full h-3 w-3 bg-blue-500"></span>
            </span>
            <div>
              <div class="flex items-center gap-2">
                <span class="text-sm font-semibold uppercase tracking-wider text-blue-900 dark:text-blue-200">
                  {{ t('ui.parserStatsView.crawlInProgress') }}
                </span>
                <span class="rounded bg-blue-200 px-2 py-0.5 text-xs font-medium text-blue-800 dark:bg-blue-900 dark:text-blue-200">
                  {{ stats.current_run.status }}
                </span>
              </div>
              <p class="mt-0.5 text-xs text-blue-700 dark:text-blue-300">
                Started: {{ formatDate(stats.current_run.started_at) }} • Last heartbeat: {{ timeAgo(stats.current_run.updated_at) }}
              </p>
            </div>
          </div>

          <div class="grid grid-cols-3 gap-4 text-center sm:text-right">
            <div>
              <p class="text-xs uppercase text-blue-600 dark:text-blue-400">{{ t('ui.parserStatsView.progress') }}</p>
              <p class="text-base font-bold text-blue-950 dark:text-blue-100">
                {{ stats.current_run.providers_done }} / {{ stats.current_run.providers_total }}
              </p>
            </div>
            <div>
              <p class="text-xs uppercase text-blue-600 dark:text-blue-400">{{ t('ui.parserStatsView.pricesExtracted') }}</p>
              <p class="text-base font-bold text-blue-950 dark:text-blue-100">
                {{ stats.current_run.prices_extracted }}
              </p>
            </div>
            <div>
              <p class="text-xs uppercase text-blue-600 dark:text-blue-400">{{ t('ui.parserStatsView.errors') }}</p>
              <p class="text-base font-bold" :class="stats.current_run.errors_count > 0 ? 'text-red-600 dark:text-red-400' : 'text-blue-950 dark:text-blue-100'">
                {{ stats.current_run.errors_count }}
              </p>
            </div>
          </div>
        </div>

        <!-- Progress Bar -->
        <div class="mt-3 h-2 w-full overflow-hidden rounded-full bg-blue-200/70 dark:bg-blue-900/60">
          <div
            class="h-full rounded-full bg-blue-600 transition-all duration-500"
            :style="{
              width: stats.current_run.providers_total > 0
                ? `${Math.min(100, Math.round((stats.current_run.providers_done / stats.current_run.providers_total) * 100))}%`
                : '10%'
            }"
          ></div>
        </div>
      </div>

      <!-- KPI Summary Cards (Totals) -->
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <!-- Models -->
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <div class="flex items-center justify-between">
            <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.totalModels') }}</p>
            <span class="rounded-lg bg-primary-50 p-2 text-primary-600 dark:bg-primary-950/40 dark:text-primary-400">
              <Icon name="grid" size="sm" />
            </span>
          </div>
          <p class="mt-2 text-3xl font-bold text-gray-900 dark:text-white">
            {{ formatNumber(stats?.totals?.models || 0) }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.discoveredcanonicalAImodels') }}</p>
        </div>

        <!-- Providers -->
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <div class="flex items-center justify-between">
            <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.totalProviders') }}</p>
            <span class="rounded-lg bg-emerald-50 p-2 text-emerald-600 dark:bg-emerald-950/40 dark:text-emerald-400">
              <Icon name="link" size="sm" />
            </span>
          </div>
          <p class="mt-2 text-3xl font-bold text-emerald-600 dark:text-emerald-400">
            {{ formatNumber(stats?.totals?.providers || 0) }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.activepricingsources') }}</p>
        </div>

        <!-- Auth Providers -->
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <div class="flex items-center justify-between">
            <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.authOnlyProviders') }}</p>
            <span class="rounded-lg bg-indigo-50 p-2 text-indigo-600 dark:bg-indigo-950/40 dark:text-indigo-400">
              <Icon name="cog" size="sm" />
            </span>
          </div>
          <p class="mt-2 text-3xl font-bold text-indigo-600 dark:text-indigo-400">
            {{ formatNumber(stats?.totals?.auth_providers || 0) }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.requiringregisteredAPIkey') }}</p>
        </div>

        <!-- Offers -->
        <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-5 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <div class="flex items-center justify-between">
            <p class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.providerModelOffers') }}</p>
            <span class="rounded-lg bg-amber-50 p-2 text-amber-600 dark:bg-amber-950/40 dark:text-amber-400">
              <Icon name="chart" size="sm" />
            </span>
          </div>
          <p class="mt-2 text-3xl font-bold text-amber-600 dark:text-amber-400">
            {{ formatNumber(stats?.totals?.offers || 0) }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.trackedmodelpricepoints') }}</p>
        </div>
      </div>

      <!-- Navigation Tabs for Sections -->
      <div class="flex border-b border-gray-200 dark:border-dark-700 overflow-x-auto">
        <button
          type="button"
          v-for="tab in tabs"
          :key="tab.id"
          class="flex items-center gap-2 border-b-2 px-5 py-3 text-sm font-medium transition-all whitespace-nowrap"
          :class="activeTab === tab.id
            ? 'border-primary-600 text-primary-600 dark:border-primary-400 dark:text-primary-400'
            : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'"
          @click="activeTab = tab.id"
        >
          <span>{{ tab.label }}</span>
          <span
            v-if="tab.badge !== undefined"
            class="rounded-full px-2 py-0.5 text-xs font-semibold"
            :class="tab.badgeClass ? tab.badgeClass : (activeTab === tab.id
              ? 'bg-primary-100 text-primary-700 dark:bg-primary-950 dark:text-primary-300'
              : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300')"
          >
            {{ tab.badge }}
          </span>
        </button>
      </div>

      <!-- TAB 1: Last Full Run -->
      <div v-if="activeTab === 'full_run'" class="space-y-6">
        <div v-if="!stats?.last_full_run" class="rounded-2xl border border-gray-200/80 bg-white/85 p-8 text-center text-sm text-gray-500 dark:border-dark-700/80 dark:bg-dark-800/85 dark:text-gray-400">
          {{ t('ui.parserStatsView.norecordedfullrun') }} <code class="font-mono">{{ t('ui.parserStatsView.updateall') }}</code> {{ t('ui.parserStatsView.pipelinecompletes') }}
        </div>

        <div v-else class="space-y-6">
          <!-- Full Run Overview Card -->
          <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-6 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
            <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between border-b border-gray-100 pb-4 dark:border-dark-700">
              <div>
                <div class="flex items-center gap-3">
                  <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('ui.parserStatsView.fullPipelineExecution') }}</h3>
                  <span
                    class="rounded-full px-3 py-1 text-xs font-semibold"
                    :class="stats.last_full_run.success
                      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300'
                      : 'bg-red-100 text-red-700 dark:bg-red-950/50 dark:text-red-300'"
                  >
                    {{ stats.last_full_run.success ? 'Success' : 'Failed' }}
                  </span>
                </div>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('ui.parserStatsView.executioncompletedin') }} <span class="font-semibold text-gray-700 dark:text-gray-200">{{ formatDuration(stats.last_full_run.duration_seconds) }}</span>
                </p>
              </div>

              <div class="text-xs text-gray-500 dark:text-gray-400">
                <div>Started: {{ formatDate(stats.last_full_run.started_at) }}</div>
                <div class="mt-0.5">Finished: {{ formatDate(stats.last_full_run.finished_at) }}</div>
              </div>
            </div>

            <!-- Full Run Detailed Metrics Grid -->
            <div class="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-7">
              <div class="rounded-xl bg-gray-50/70 p-4 text-center dark:bg-dark-900/40">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.providersFound') }}</p>
                <p class="mt-1 text-xl font-bold text-gray-900 dark:text-white">{{ stats.last_full_run.providers_found }}</p>
              </div>
              <div class="rounded-xl bg-gray-50/70 p-4 text-center dark:bg-dark-900/40">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.uniqueProviders') }}</p>
                <p class="mt-1 text-xl font-bold text-gray-900 dark:text-white">{{ stats.last_full_run.providers_unique }}</p>
              </div>
              <div class="rounded-xl bg-gray-50/70 p-4 text-center dark:bg-dark-900/40">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.pricingPages') }}</p>
                <p class="mt-1 text-xl font-bold text-gray-900 dark:text-white">{{ stats.last_full_run.pricing_pages_found }}</p>
              </div>
              <div class="rounded-xl bg-gray-50/70 p-4 text-center dark:bg-dark-900/40">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.pricesExtracted2') }}</p>
                <p class="mt-1 text-xl font-bold text-primary-600 dark:text-primary-400">{{ stats.last_full_run.prices_extracted }}</p>
              </div>
              <div class="rounded-xl bg-gray-50/70 p-4 text-center dark:bg-dark-900/40">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.pricesPublished') }}</p>
                <p class="mt-1 text-xl font-bold text-emerald-600 dark:text-emerald-400">{{ stats.last_full_run.prices_published }}</p>
              </div>
              <div class="rounded-xl bg-gray-50/70 p-4 text-center dark:bg-dark-900/40">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.needingReview') }}</p>
                <p class="mt-1 text-xl font-bold" :class="stats.last_full_run.records_needing_review > 0 ? 'text-amber-600 dark:text-amber-400' : 'text-gray-900 dark:text-white'">
                  {{ stats.last_full_run.records_needing_review }}
                </p>
              </div>
              <div class="rounded-xl bg-gray-50/70 p-4 text-center dark:bg-dark-900/40">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.errors2') }}</p>
                <p class="mt-1 text-xl font-bold" :class="stats.last_full_run.errors_count > 0 ? 'text-red-600 dark:text-red-400' : 'text-emerald-600 dark:text-emerald-400'">
                  {{ stats.last_full_run.errors_count }}
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- TAB 2: Last Change / Diff -->
      <div v-if="activeTab === 'last_change'" class="space-y-6">
        <div v-if="!stats?.last_change" class="rounded-2xl border border-gray-200/80 bg-white/85 p-8 text-center text-sm text-gray-500 dark:border-dark-700/80 dark:bg-dark-800/85 dark:text-gray-400">
          {{ t('ui.parserStatsView.nochangesrecordedyet') }}
        </div>

        <div v-else class="space-y-6">
          <!-- Change Header -->
          <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-6 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
            <div class="flex flex-wrap items-center justify-between gap-4">
              <div>
                <div class="flex items-center gap-3">
                  <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('ui.parserStatsView.recentSyncDiff') }}</h3>
                  <span
                    v-if="stats.last_change.baseline"
                    class="rounded-full bg-blue-100 px-3 py-1 text-xs font-semibold text-blue-700 dark:bg-blue-950/50 dark:text-blue-300"
                  >
                    {{ t('ui.parserStatsView.baselineEstablished') }}
                  </span>
                  <span
                    v-if="stats.last_change.in_progress"
                    class="rounded-full bg-amber-100 px-3 py-1 text-xs font-semibold text-amber-700 dark:bg-amber-950/50 dark:text-amber-300"
                  >
                    {{ t('ui.parserStatsView.inProgress') }}
                  </span>
                </div>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  Recorded at: {{ formatDate(stats.last_change.at) }}
                  <span v-if="stats.last_change.at">({{ timeAgo(stats.last_change.at) }})</span>
                </p>
              </div>

              <div class="flex items-center gap-2">
                <span class="rounded-xl bg-primary-50 px-3 py-1.5 text-xs font-semibold text-primary-700 dark:bg-primary-950/50 dark:text-primary-300">
                  +{{ stats.last_change.new_offers_count }} New Offers
                </span>
              </div>
            </div>

            <!-- Summary Chips -->
            <div class="mt-6 flex flex-wrap gap-3">
              <span class="inline-flex items-center gap-1.5 rounded-lg border border-emerald-200 bg-emerald-50/60 px-3 py-1.5 text-xs font-medium text-emerald-800 dark:border-emerald-900/40 dark:bg-emerald-950/20 dark:text-emerald-300">
                <span>{{ t('ui.parserStatsView.newModels') }}</span>
                <span class="font-bold">{{ stats.last_change.new_models?.length || 0 }}</span>
              </span>
              <span class="inline-flex items-center gap-1.5 rounded-lg border border-emerald-200 bg-emerald-50/60 px-3 py-1.5 text-xs font-medium text-emerald-800 dark:border-emerald-900/40 dark:bg-emerald-950/20 dark:text-emerald-300">
                <span>{{ t('ui.parserStatsView.newProviders') }}</span>
                <span class="font-bold">{{ stats.last_change.new_providers?.length || 0 }}</span>
              </span>
              <span class="inline-flex items-center gap-1.5 rounded-lg border border-red-200 bg-red-50/60 px-3 py-1.5 text-xs font-medium text-red-800 dark:border-red-900/40 dark:bg-red-950/20 dark:text-red-300">
                <span>{{ t('ui.parserStatsView.removedModels') }}</span>
                <span class="font-bold">{{ stats.last_change.removed_models?.length || 0 }}</span>
              </span>
              <span class="inline-flex items-center gap-1.5 rounded-lg border border-red-200 bg-red-50/60 px-3 py-1.5 text-xs font-medium text-red-800 dark:border-red-900/40 dark:bg-red-950/20 dark:text-red-300">
                <span>{{ t('ui.parserStatsView.removedProviders') }}</span>
                <span class="font-bold">{{ stats.last_change.removed_providers?.length || 0 }}</span>
              </span>
            </div>
          </div>

          <!-- New Models Table -->
          <div v-if="stats.last_change.new_models && stats.last_change.new_models.length > 0" class="overflow-hidden rounded-2xl border border-gray-200/80 bg-white/85 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
            <div class="border-b border-gray-200 px-6 py-3.5 dark:border-dark-700">
              <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
                Newly Added Models ({{ stats.last_change.new_models.length }})
              </h4>
            </div>
            <div class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
                <thead class="bg-gray-50/60 text-left text-xs uppercase tracking-wider text-gray-500 dark:bg-dark-900/40 dark:text-gray-400">
                  <tr>
                    <th class="px-6 py-3">{{ t('ui.parserStatsView.canonicalModelID') }}</th>
                    <th class="px-6 py-3">{{ t('ui.parserStatsView.displayName') }}</th>
                    <th class="px-6 py-3">{{ t('ui.parserStatsView.servingProviders') }}</th>
                    <th class="px-6 py-3 text-right">{{ t('ui.parserStatsView.requiresAuth') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-50 dark:divide-dark-800">
                  <tr v-for="m in stats.last_change.new_models" :key="m.canonical_model_id">
                    <td class="px-6 py-3 font-mono text-xs font-semibold text-primary-600 dark:text-primary-400">
                      {{ m.canonical_model_id }}
                    </td>
                    <td class="px-6 py-3 text-gray-900 dark:text-white">
                      {{ m.display_name }}
                    </td>
                    <td class="px-6 py-3 text-xs text-gray-500 dark:text-gray-400">
                      {{ (m.providers || []).join(', ') || '—' }}
                    </td>
                    <td class="px-6 py-3 text-right">
                      <span
                        v-if="m.via_auth_provider"
                        class="rounded bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800 dark:bg-amber-950/60 dark:text-amber-300"
                      >
                        Yes
                      </span>
                      <span v-else class="text-xs text-gray-400">No</span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- New Providers Table -->
          <div v-if="stats.last_change.new_providers && stats.last_change.new_providers.length > 0" class="overflow-hidden rounded-2xl border border-gray-200/80 bg-white/85 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
            <div class="border-b border-gray-200 px-6 py-3.5 dark:border-dark-700">
              <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
                Newly Added Providers ({{ stats.last_change.new_providers.length }})
              </h4>
            </div>
            <div class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
                <thead class="bg-gray-50/60 text-left text-xs uppercase tracking-wider text-gray-500 dark:bg-dark-900/40 dark:text-gray-400">
                  <tr>
                    <th class="px-6 py-3">{{ t('ui.parserStatsView.providerName') }}</th>
                    <th class="px-6 py-3">{{ t('ui.parserStatsView.domain') }}</th>
                    <th class="px-6 py-3 text-right">{{ t('ui.parserStatsView.requiresAuth2') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-50 dark:divide-dark-800">
                  <tr v-for="p in stats.last_change.new_providers" :key="p.provider_domain">
                    <td class="px-6 py-3 font-medium text-gray-900 dark:text-white">
                      {{ p.provider_name }}
                    </td>
                    <td class="px-6 py-3 font-mono text-xs text-gray-500 dark:text-gray-400">
                      {{ p.provider_domain }}
                    </td>
                    <td class="px-6 py-3 text-right">
                      <span
                        v-if="p.requires_auth"
                        class="rounded bg-indigo-100 px-2 py-0.5 text-xs font-medium text-indigo-800 dark:bg-indigo-950/60 dark:text-indigo-300"
                      >
                        {{ t('ui.parserStatsView.authOnly') }}
                      </span>
                      <span v-else class="text-xs text-gray-400">{{ t('ui.parserStatsView.public') }}</span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- Removed Models & Providers lists -->
          <div
            v-if="(stats.last_change.removed_models && stats.last_change.removed_models.length > 0) || (stats.last_change.removed_providers && stats.last_change.removed_providers.length > 0)"
            class="grid grid-cols-1 gap-6 md:grid-cols-2"
          >
            <div v-if="stats.last_change.removed_models && stats.last_change.removed_models.length > 0" class="rounded-2xl border border-red-200/80 bg-red-50/20 p-5 dark:border-red-950/40 dark:bg-red-950/10">
              <h5 class="text-xs font-semibold uppercase tracking-wider text-red-700 dark:text-red-400">
                Removed Models ({{ stats.last_change.removed_models.length }})
              </h5>
              <div class="mt-3 flex flex-wrap gap-2">
                <span
                  v-for="rm in stats.last_change.removed_models"
                  :key="rm"
                  class="rounded bg-red-100/80 px-2.5 py-1 font-mono text-xs text-red-800 dark:bg-red-950/60 dark:text-red-300"
                >
                  {{ rm }}
                </span>
              </div>
            </div>

            <div v-if="stats.last_change.removed_providers && stats.last_change.removed_providers.length > 0" class="rounded-2xl border border-red-200/80 bg-red-50/20 p-5 dark:border-red-950/40 dark:bg-red-950/10">
              <h5 class="text-xs font-semibold uppercase tracking-wider text-red-700 dark:text-red-400">
                Removed Providers ({{ stats.last_change.removed_providers.length }})
              </h5>
              <div class="mt-3 flex flex-wrap gap-2">
                <span
                  v-for="rp in stats.last_change.removed_providers"
                  :key="rp"
                  class="rounded bg-red-100/80 px-2.5 py-1 font-mono text-xs text-red-800 dark:bg-red-950/60 dark:text-red-300"
                >
                  {{ rp }}
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- TAB 3: History -->
      <div v-if="activeTab === 'history'" class="space-y-6">
        <div v-if="!stats?.history || stats.history.length === 0" class="rounded-2xl border border-gray-200/80 bg-white/85 p-8 text-center text-sm text-gray-500 dark:border-dark-700/80 dark:bg-dark-800/85 dark:text-gray-400">
          {{ t('ui.parserStatsView.nohistoryentriesyet') }}
        </div>

        <div v-else class="overflow-hidden rounded-2xl border border-gray-200/80 bg-white/85 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
          <div class="border-b border-gray-200 px-6 py-3.5 dark:border-dark-700">
            <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
              Crawl Change History ({{ stats.history.length }} runs)
            </h4>
          </div>
          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
              <thead class="bg-gray-50/60 text-left text-xs uppercase tracking-wider text-gray-500 dark:bg-dark-900/40 dark:text-gray-400">
                <tr>
                  <th class="px-6 py-3">{{ t('ui.parserStatsView.timestamp') }}</th>
                  <th class="px-6 py-3">{{ t('ui.parserStatsView.type') }}</th>
                  <th class="px-6 py-3 text-right">{{ t('ui.parserStatsView.newModels2') }}</th>
                  <th class="px-6 py-3 text-right">{{ t('ui.parserStatsView.newProviders2') }}</th>
                  <th class="px-6 py-3 text-right">{{ t('ui.parserStatsView.removed') }}</th>
                  <th class="px-6 py-3 text-right">{{ t('ui.parserStatsView.newOffers') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-50 dark:divide-dark-800">
                <tr v-for="(h, idx) in stats.history" :key="idx">
                  <td class="px-6 py-3.5">
                    <div class="font-medium text-gray-900 dark:text-white">{{ formatDate(h.at) }}</div>
                    <div class="text-xs text-gray-400">{{ timeAgo(h.at) }}</div>
                  </td>
                  <td class="px-6 py-3.5">
                    <span
                      v-if="h.baseline"
                      class="rounded-full bg-blue-100 px-2.5 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-950/50 dark:text-blue-300"
                    >
                      {{ t('ui.parserStatsView.baseline') }}
                    </span>
                    <span
                      v-else
                      class="rounded-full bg-gray-100 px-2.5 py-0.5 text-xs font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-300"
                    >
                      {{ t('ui.parserStatsView.diff') }}
                    </span>
                  </td>
                  <td class="px-6 py-3.5 text-right font-semibold" :class="h.new_models?.length ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-400'">
                    {{ h.new_models?.length || 0 }}
                  </td>
                  <td class="px-6 py-3.5 text-right font-semibold" :class="h.new_providers?.length ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-400'">
                    {{ h.new_providers?.length || 0 }}
                  </td>
                  <td class="px-6 py-3.5 text-right font-semibold" :class="(h.removed_models?.length || 0) + (h.removed_providers?.length || 0) > 0 ? 'text-red-600 dark:text-red-400' : 'text-gray-400'">
                    {{ (h.removed_models?.length || 0) + (h.removed_providers?.length || 0) }}
                  </td>
                  <td class="px-6 py-3.5 text-right font-semibold text-primary-600 dark:text-primary-400">
                    +{{ h.new_offers_count || 0 }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <!-- TAB 4: Official Prices -->
      <div v-if="activeTab === 'official_prices'" class="space-y-6">
        <div v-if="!stats?.official_prices" class="rounded-2xl border border-gray-200/80 bg-white/85 p-8 text-center text-sm text-gray-500 dark:border-dark-700/80 dark:bg-dark-800/85 dark:text-gray-400">
          {{ t('ui.parserStatsView.noofficialpricingtelemetry') }}
        </div>

        <div v-else class="space-y-6">
          <!-- Overview Card -->
          <div class="rounded-2xl border border-gray-200/80 bg-white/85 p-6 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
            <div class="flex flex-wrap items-center justify-between gap-4">
              <div>
                <h3 class="text-base font-semibold text-gray-900 dark:text-white">
                  {{ t('ui.parserStatsView.officialProviderPricingVerificatio') }}
                </h3>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  Audit log of canonical provider price changes, detected price spikes/drops, and verification history.
                  <span v-if="stats.official_prices.at" class="ml-1">
                    Recorded at {{ formatDate(stats.official_prices.at) }} ({{ timeAgo(stats.official_prices.at) }})
                  </span>
                </p>
              </div>
            </div>

            <!-- Metric summary cards -->
            <div class="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-4">
              <div class="rounded-xl border border-emerald-100 bg-emerald-50/50 p-4 text-center dark:border-emerald-900/40 dark:bg-emerald-950/20">
                <p class="text-xs font-medium text-emerald-700 dark:text-emerald-400">{{ t('ui.parserStatsView.newlyAdded') }}</p>
                <p class="mt-1 text-2xl font-bold text-emerald-800 dark:text-emerald-300">
                  {{ stats.official_prices.added?.length || 0 }}
                </p>
              </div>
              <div class="rounded-xl border border-blue-100 bg-blue-50/50 p-4 text-center dark:border-blue-900/40 dark:bg-blue-950/20">
                <p class="text-xs font-medium text-blue-700 dark:text-blue-400">{{ t('ui.parserStatsView.priceUpdates') }}</p>
                <p class="mt-1 text-2xl font-bold text-blue-800 dark:text-blue-300">
                  {{ stats.official_prices.updated?.length || 0 }}
                </p>
              </div>
              <div
                class="rounded-xl border p-4 text-center"
                :class="(stats.official_prices.suspicious?.length || 0) > 0
                  ? 'border-amber-200 bg-amber-50/70 dark:border-amber-900/40 dark:bg-amber-950/20'
                  : 'border-gray-100 bg-gray-50/70 dark:border-dark-700 dark:bg-dark-900/40'"
              >
                <p
                  class="text-xs font-medium"
                  :class="(stats.official_prices.suspicious?.length || 0) > 0 ? 'text-amber-800 dark:text-amber-400' : 'text-gray-500 dark:text-gray-400'"
                >
                  {{ t('ui.parserStatsView.suspiciousFlags') }}
                </p>
                <p
                  class="mt-1 text-2xl font-bold"
                  :class="(stats.official_prices.suspicious?.length || 0) > 0 ? 'text-amber-700 dark:text-amber-300' : 'text-gray-900 dark:text-white'"
                >
                  {{ stats.official_prices.suspicious?.length || 0 }}
                </p>
              </div>
              <div class="rounded-xl border border-gray-100 bg-gray-50/70 p-4 text-center dark:border-dark-700 dark:bg-dark-900/40">
                <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('ui.parserStatsView.historyRecords') }}</p>
                <p class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">
                  {{ stats.official_prices.history?.length || 0 }}
                </p>
              </div>
            </div>
          </div>

          <!-- Suspicious Price Items Section -->
          <div
            v-if="stats.official_prices.suspicious && stats.official_prices.suspicious.length > 0"
            class="overflow-hidden rounded-2xl border border-amber-200 bg-amber-50/30 shadow-sm dark:border-amber-900/40 dark:bg-amber-950/10"
          >
            <div class="flex items-center gap-2 border-b border-amber-200/60 px-6 py-3.5 dark:border-amber-900/40">
              <Icon name="exclamationTriangle" size="sm" class="text-amber-600 dark:text-amber-400" />
              <h4 class="text-sm font-semibold text-amber-900 dark:text-amber-200">
                Suspicious Price Changes Flagged ({{ stats.official_prices.suspicious.length }})
              </h4>
            </div>
            <div class="p-4 space-y-3">
              <div
                v-for="(item, idx) in stats.official_prices.suspicious"
                :key="idx"
                class="rounded-xl border border-amber-200/80 bg-white p-4 shadow-xs dark:border-amber-900/30 dark:bg-dark-800"
              >
                <pre v-if="typeof item === 'object'" class="font-mono text-xs text-gray-800 dark:text-gray-200 overflow-x-auto">{{ JSON.stringify(item, null, 2) }}</pre>
                <p v-else class="text-sm text-gray-800 dark:text-gray-200">{{ item }}</p>
              </div>
            </div>
          </div>

          <!-- Added Prices Section -->
          <div
            v-if="stats.official_prices.added && stats.official_prices.added.length > 0"
            class="overflow-hidden rounded-2xl border border-gray-200/80 bg-white/85 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85"
          >
            <div class="border-b border-gray-200 px-6 py-3.5 dark:border-dark-700">
              <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
                Newly Added Official Prices ({{ stats.official_prices.added.length }})
              </h4>
            </div>
            <div class="p-4 space-y-3">
              <div
                v-for="(item, idx) in stats.official_prices.added"
                :key="idx"
                class="rounded-xl border border-gray-100 bg-gray-50/50 p-4 dark:border-dark-700 dark:bg-dark-900/40"
              >
                <pre v-if="typeof item === 'object'" class="font-mono text-xs text-gray-800 dark:text-gray-200 overflow-x-auto">{{ JSON.stringify(item, null, 2) }}</pre>
                <p v-else class="text-sm text-gray-800 dark:text-gray-200">{{ item }}</p>
              </div>
            </div>
          </div>

          <!-- Updated Prices Section -->
          <div
            v-if="stats.official_prices.updated && stats.official_prices.updated.length > 0"
            class="overflow-hidden rounded-2xl border border-gray-200/80 bg-white/85 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85"
          >
            <div class="border-b border-gray-200 px-6 py-3.5 dark:border-dark-700">
              <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
                Updated Official Prices ({{ stats.official_prices.updated.length }})
              </h4>
            </div>
            <div class="p-4 space-y-3">
              <div
                v-for="(item, idx) in stats.official_prices.updated"
                :key="idx"
                class="rounded-xl border border-gray-100 bg-gray-50/50 p-4 dark:border-dark-700 dark:bg-dark-900/40"
              >
                <pre v-if="typeof item === 'object'" class="font-mono text-xs text-gray-800 dark:text-gray-200 overflow-x-auto">{{ JSON.stringify(item, null, 2) }}</pre>
                <p v-else class="text-sm text-gray-800 dark:text-gray-200">{{ item }}</p>
              </div>
            </div>
          </div>

          <!-- Empty state when no items in added/updated/suspicious -->
          <div
            v-if="(!stats.official_prices.added || stats.official_prices.added.length === 0) &&
                   (!stats.official_prices.updated || stats.official_prices.updated.length === 0) &&
                   (!stats.official_prices.suspicious || stats.official_prices.suspicious.length === 0)"
            class="rounded-2xl border border-gray-200/80 bg-white/85 p-8 text-center text-sm text-gray-500 dark:border-dark-700/80 dark:bg-dark-800/85 dark:text-gray-400"
          >
            <Icon name="checkCircle" size="lg" class="mx-auto mb-2 text-emerald-500" />
            <p class="font-medium text-gray-900 dark:text-white">{{ t('ui.parserStatsView.allOfficialPricesUp') }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">No added, updated, or suspicious official pricing changes detected in this synchronization run.</p>
          </div>
        </div>
      </div>

      <!-- TAB 5: Errors -->
      <div v-if="activeTab === 'errors'" class="space-y-6">
        <div v-if="!stats?.errors || stats.errors.length === 0" class="rounded-2xl border border-emerald-200/70 bg-emerald-50/40 p-8 text-center dark:border-emerald-900/40 dark:bg-emerald-950/20">
          <Icon name="checkCircle" size="lg" class="mx-auto mb-2 text-emerald-600 dark:text-emerald-400" />
          <h4 class="text-base font-semibold text-emerald-950 dark:text-emerald-200">{{ t('ui.parserStatsView.zeroParserErrors') }}</h4>
          <p class="mt-1 text-xs text-emerald-700 dark:text-emerald-400">
            All crawlers, network fetches, and pricing page processors finished cleanly without reported exceptions.
          </p>
        </div>

        <div v-else class="space-y-4">
          <div class="rounded-2xl border border-red-200 bg-red-50/60 p-4 dark:border-red-900/50 dark:bg-red-950/20">
            <div class="flex items-center gap-3">
              <Icon name="exclamationTriangle" size="md" class="text-red-600 dark:text-red-400" />
              <div>
                <h4 class="text-sm font-semibold text-red-900 dark:text-red-200">
                  {{ stats.errors.length }} {{ stats.errors.length === 1 ? 'Error' : 'Errors' }} Encountered
                </h4>
                <p class="text-xs text-red-700 dark:text-red-300">
                  {{ t('ui.parserStatsView.crawlerReportedFailures') }}
                </p>
              </div>
            </div>
          </div>

          <div class="space-y-3">
            <div
              v-for="(err, idx) in stats.errors"
              :key="idx"
              class="rounded-xl border border-gray-200/80 bg-white/90 p-4 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/90"
            >
              <div class="flex items-start justify-between gap-3">
                <span class="rounded bg-red-100 px-2 py-0.5 font-mono text-xs font-semibold text-red-800 dark:bg-red-950/70 dark:text-red-300">
                  #{{ idx + 1 }}
                </span>
              </div>
              <div class="mt-2">
                <pre v-if="typeof err === 'object'" class="font-mono text-xs leading-relaxed text-red-700 dark:text-red-300 overflow-x-auto">{{ JSON.stringify(err, null, 2) }}</pre>
                <p v-else class="font-mono text-xs leading-relaxed text-red-700 dark:text-red-300">{{ err }}</p>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Raw JSON Modal -->
      <Teleport to="body">
        <div v-if="showRawModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
          <div class="relative w-full max-w-4xl max-h-[85vh] flex flex-col rounded-2xl border border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-800">
            <div class="flex items-center justify-between border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h3 class="text-base font-semibold text-gray-900 dark:text-white">
                {{ t('ui.parserStatsView.rawRunStatsContent') }}
              </h3>
              <div class="flex items-center gap-2">
                <button
                  type="button"
                  class="rounded-lg border border-gray-200 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-700 dark:text-gray-300 dark:hover:bg-dark-700"
                  @click="copyRawJSON"
                >
                  {{ copySuccess ? t('ui.parserStatsView.copied') : t('ui.parserStatsView.copyJson') }}
                </button>
                <button
                  type="button"
                  class="rounded-lg p-1.5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
                  @click="showRawModal = false"
                >
                  <Icon name="x" size="sm" />
                </button>
              </div>
            </div>
            <div class="flex-1 overflow-auto p-6">
              <pre class="font-mono text-xs leading-relaxed text-gray-800 dark:text-gray-200 bg-gray-50 dark:bg-dark-900 p-4 rounded-xl overflow-x-auto">{{ rawJSONString }}</pre>
            </div>
          </div>
        </div>
      </Teleport>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { getParserRunStats, type RunStatsData } from '@/api/admin/parserStats'

const { t } = useI18n()

const loading = ref(false)
const stats = ref<RunStatsData | null>(null)
const activeTab = ref<'full_run' | 'last_change' | 'official_prices' | 'history' | 'errors'>('full_run')
const showRawModal = ref(false)
const copySuccess = ref(false)

const tabs = computed(() => [
  {
    id: 'full_run' as const,
    label: 'Last Full Run',
    badge: stats.value?.last_full_run ? (stats.value.last_full_run.success ? '✓' : '✗') : undefined,
    badgeClass: stats.value?.last_full_run
      ? (stats.value.last_full_run.success
          ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-300'
          : 'bg-red-100 text-red-700 dark:bg-red-950/60 dark:text-red-300')
      : undefined
  },
  {
    id: 'last_change' as const,
    label: 'Recent Changes',
    badge: stats.value?.last_change?.new_models?.length ? `+${stats.value.last_change.new_models.length}` : undefined,
    badgeClass: undefined
  },
  {
    id: 'official_prices' as const,
    label: 'Official Prices',
    badge: stats.value?.official_prices
      ? ((stats.value.official_prices.added?.length || 0) +
         (stats.value.official_prices.updated?.length || 0) +
         (stats.value.official_prices.suspicious?.length || 0)) || undefined
      : undefined,
    badgeClass: (stats.value?.official_prices?.suspicious?.length || 0) > 0
      ? 'bg-amber-100 text-amber-800 dark:bg-amber-950/60 dark:text-amber-300'
      : undefined
  },
  {
    id: 'history' as const,
    label: 'Change History',
    badge: stats.value?.history?.length || 0,
    badgeClass: undefined
  },
  {
    id: 'errors' as const,
    label: 'Errors',
    badge: stats.value?.errors !== undefined ? stats.value.errors.length : undefined,
    badgeClass: (stats.value?.errors?.length || 0) > 0
      ? 'bg-red-100 text-red-700 dark:bg-red-950/60 dark:text-red-300'
      : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-300'
  }
])

const rawJSONString = computed(() => {
  if (!stats.value) return ''
  return JSON.stringify(stats.value, null, 2)
})

async function fetchStats() {
  loading.value = true
  try {
    stats.value = await getParserRunStats()
  } catch (err) {
    console.error('Failed to load parser run stats:', err)
  } finally {
    loading.value = false
  }
}

function formatDate(dateStr?: string | null): string {
  if (!dateStr) return '—'
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return dateStr
  return d.toLocaleString()
}

function timeAgo(dateStr?: string | null): string {
  if (!dateStr) return '—'
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return ''
  const diffSec = Math.floor((Date.now() - d.getTime()) / 1000)
  if (diffSec < 60) return t('common.time.justNow')
  const diffMin = Math.floor(diffSec / 60)
  if (diffMin < 60) return t('common.time.minutesAgo', { n: diffMin })
  const diffHours = Math.floor(diffMin / 60)
  if (diffHours < 24) return t('common.time.hoursAgo', { n: diffHours })
  const diffDays = Math.floor(diffHours / 24)
  return t('common.time.daysAgo', { n: diffDays })
}

function formatDuration(seconds?: number): string {
  if (seconds === undefined || seconds === null) return '—'
  if (seconds < 60) return `${seconds.toFixed(1)}s`
  const mins = Math.floor(seconds / 60)
  const remSec = Math.round(seconds % 60)
  return `${mins}m ${remSec}s`
}

function formatNumber(num: number): string {
  return new Intl.NumberFormat().format(num)
}

async function copyRawJSON() {
  try {
    await navigator.clipboard.writeText(rawJSONString.value)
    copySuccess.value = true
    setTimeout(() => {
      copySuccess.value = false
    }, 2000)
  } catch (e) {
    console.error('Failed to copy to clipboard', e)
  }
}

onMounted(() => {
  fetchStats()
})
</script>
