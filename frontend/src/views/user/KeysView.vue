<template>
  <AppLayout>
    <TablePageLayout page-scroll>
      <template v-if="isAdmin" #filters>
        <div class="flex flex-wrap items-center gap-3">
          <SearchInput
            v-model="filterSearch"
            :placeholder="t('keys.searchPlaceholder')"
            class="w-full sm:w-64"
            @search="onFilterChange"
          />
          <Select
            v-if="isAdmin"
            :model-value="filterUserId"
            class="w-40"
            :options="userFilterOptions"
            @update:model-value="onUserFilterChange"
          />
          <Select
            :model-value="filterGroupId"
            class="w-40"
            :options="groupFilterOptions"
            @update:model-value="onGroupFilterChange"
          />
          <Select
            :model-value="filterStatus"
            class="w-40"
            :options="statusFilterOptions"
            @update:model-value="onStatusFilterChange"
          />
        </div>
      </template>

      <template #actions>
        <div class="flex justify-end gap-3">
        <button
          v-if="isAdmin"
          @click="loadApiKeys"
          :disabled="loading"
          class="btn btn-secondary"
          :title="t('common.refresh')"
        >
          <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
        </button>
        <button
          @click="handleCreateKeyClick"
          class="btn btn-primary"
          data-tour="keys-create-btn"
        >
          <Icon name="plus" size="md" class="mr-2" />
          {{ t('keys.createKey') }}
        </button>
      </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="apiKeys" :loading="loading" :scrollable="false">
          <template #cell-key="{ value, row }">
            <div class="flex items-center gap-1">
              <code
                class="code inline-block max-w-[160px] truncate text-sm"
                :title="revealedKeys[row.id] || undefined"
              >
                {{ displayKey(row, value) }}
              </code>
              <template v-if="isRevealable(row)">
                <button
                  :disabled="revealingKeyId === row.id"
                  @click="toggleReveal(row)"
                  class="rounded-lg p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-300"
                  :title="revealedKeys[row.id] ? t('keys.hideKey') : t('keys.revealKey')"
                >
                  <Icon :name="revealedKeys[row.id] ? 'eyeOff' : 'eye'" size="sm" />
                </button>
                <button
                  @click="copyFullKey(row)"
                  class="rounded-lg p-1 transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
                  :class="
                    copiedKeyId === row.id
                      ? 'text-green-500'
                      : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'
                  "
                  :title="copiedKeyId === row.id ? t('keys.copied') : t('keys.copyToClipboard')"
                >
                  <Icon
                    v-if="copiedKeyId === row.id"
                    name="check"
                    size="sm"
                    :stroke-width="2"
                  />
                  <Icon v-else name="clipboard" size="sm" />
                </button>
              </template>
              <span
                v-else
                class="whitespace-nowrap text-sm text-gray-400 dark:text-dark-500"
                :title="t('keys.keyNotRecoverable')"
              >
                {{ t('keys.keyNotRecoverableShort') }}
              </span>
            </div>
          </template>

          <template #cell-owner="{ row }">
            <div v-if="isAdmin" class="flex flex-col leading-5">
              <span class="max-w-[110px] truncate text-sm font-medium text-gray-900 dark:text-white">
                {{ row.user?.username || row.user?.email || 'N/A' }}
              </span>
              <span class="text-xs text-gray-500">ID: {{ row.user_id }}</span>
            </div>
          </template>

          <template #cell-name="{ value, row }">
            <div class="flex items-center gap-1.5">
              <span
                class="min-w-0 whitespace-normal break-words text-base font-medium text-gray-900 dark:text-white"
                :title="value"
              >{{ value }}</span>
              <Icon
                v-if="row.ip_whitelist?.length > 0 || row.ip_blacklist?.length > 0"
                name="shield"
                size="sm"
                class="shrink-0 text-blue-500"
                :title="t('keys.ipRestrictionEnabled')"
              />
            </div>
          </template>

          <!-- The whole cell is the group picker: clicking it opens the group
               dropdown. No "Edit" label / chevron — the column has to stay narrow. -->
          <template #cell-group="{ row }">
            <button
              :ref="(el) => setGroupButtonRef(row.id, el)"
              @click="editKey(row)"
              class="group/dropdown -mx-2 -my-1 flex cursor-pointer items-center gap-1 rounded-lg px-2 py-1 transition-all duration-200 hover:bg-gray-100 dark:hover:bg-dark-700"
              :title="t('keys.clickToChangeGroup')"
            >
              <div v-if="(row.groups && row.groups.length > 0) || row.group" class="flex items-center gap-1">
                <GroupBadge
                  v-for="group in ((row.groups && row.groups.length > 0) ? row.groups.slice(0, 1) : (row.group ? [row.group] : []))"
                  :key="group.id"
                  :name="group.name"
                  :platform="group.platform"
                  :subscription-type="group.subscription_type"
                  :rate-multiplier="group.rate_multiplier"
                  :user-rate-multiplier="userGroupRates[group.id]"
                />
                <span
                  v-if="row.groups && row.groups.length > 1"
                  class="rounded-full bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-300"
                >
                  +{{ row.groups.length - 1 }}
                </span>
              </div>
              <span v-else class="text-base text-gray-400 dark:text-dark-500">{{
                t('keys.noGroup')
              }}</span>
            </button>
          </template>

          <!-- Spending cap of the key (OpenLux "Limit"): the cap plus how much of
               it is already consumed. Unlimited keys show a dash. -->
          <template #cell-limit="{ row }">
            <div v-if="row.quota > 0" class="w-[58px]">
              <div class="text-sm leading-5 tabular-nums">
                <div :class="[
                  'font-medium',
                  row.quota_used >= row.quota ? 'text-red-500' :
                  row.quota_used >= row.quota * 0.8 ? 'text-yellow-600' :
                  'text-gray-900 dark:text-white'
                ]">
                  ${{ row.quota_used?.toFixed(2) || '0.00' }}
                </div>
                <div class="text-gray-400 dark:text-dark-500">${{ row.quota?.toFixed(2) }}</div>
              </div>
              <div class="mt-1 h-1 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  :class="[
                    'h-full rounded-full transition-all',
                    row.quota_used >= row.quota ? 'bg-red-500' :
                    row.quota_used >= row.quota * 0.8 ? 'bg-yellow-500' :
                    'bg-primary-500'
                  ]"
                  :style="{ width: Math.min((row.quota_used / row.quota) * 100, 100) + '%' }"
                />
              </div>
            </div>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">{{ t('keys.unlimited') }}</span>
          </template>

          <!-- Spend. Label above value keeps the column narrow (the whole table
               has to fit the card without a horizontal scrollbar). -->
          <template #cell-usage="{ row }">
            <div class="text-sm leading-5 tabular-nums">
              <div class="text-gray-400 dark:text-dark-500">{{ t('keys.today') }}</div>
              <div class="font-medium text-gray-900 dark:text-white">
                ${{ (usageStats[row.id]?.today_actual_cost ?? 0).toFixed(4) }}
              </div>
              <div class="mt-1 text-gray-400 dark:text-dark-500">{{ t('keys.total') }}</div>
              <div class="font-medium text-gray-900 dark:text-white">
                ${{ (usageStats[row.id]?.total_actual_cost ?? 0).toFixed(4) }}
              </div>
            </div>
          </template>

          <template #cell-rate_limit="{ row }">
            <div v-if="row.rate_limit_5h > 0 || row.rate_limit_1d > 0 || row.rate_limit_7d > 0" class="w-[88px] space-y-1">
              <!-- 5h window -->
              <div v-if="row.rate_limit_5h > 0">
                <div class="flex items-center justify-between text-sm">
                  <span class="text-gray-500 dark:text-gray-400">5h</span>
                  <span :class="[
                    'font-medium tabular-nums',
                    row.usage_5h >= row.rate_limit_5h ? 'text-red-500' :
                    row.usage_5h >= row.rate_limit_5h * 0.8 ? 'text-yellow-500' :
                    'text-gray-700 dark:text-gray-300'
                  ]">
                    ${{ row.usage_5h?.toFixed(2) || '0.00' }}/${{ row.rate_limit_5h?.toFixed(2) }}
                  </span>
                </div>
                <div class="h-1 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-all',
                      row.usage_5h >= row.rate_limit_5h ? 'bg-red-500' :
                      row.usage_5h >= row.rate_limit_5h * 0.8 ? 'bg-yellow-500' :
                      'bg-emerald-500'
                    ]"
                    :style="{ width: Math.min((row.usage_5h / row.rate_limit_5h) * 100, 100) + '%' }"
                  />
                </div>
                <div v-if="row.reset_5h_at && formatResetTime(row.reset_5h_at)" class="text-xs text-gray-400 dark:text-gray-500 tabular-nums">
                  ⟳ {{ formatResetTime(row.reset_5h_at) }}
                </div>
              </div>
              <!-- 1d window -->
              <div v-if="row.rate_limit_1d > 0">
                <div class="flex items-center justify-between text-sm">
                  <span class="text-gray-500 dark:text-gray-400">1d</span>
                  <span :class="[
                    'font-medium tabular-nums',
                    row.usage_1d >= row.rate_limit_1d ? 'text-red-500' :
                    row.usage_1d >= row.rate_limit_1d * 0.8 ? 'text-yellow-500' :
                    'text-gray-700 dark:text-gray-300'
                  ]">
                    ${{ row.usage_1d?.toFixed(2) || '0.00' }}/${{ row.rate_limit_1d?.toFixed(2) }}
                  </span>
                </div>
                <div class="h-1 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-all',
                      row.usage_1d >= row.rate_limit_1d ? 'bg-red-500' :
                      row.usage_1d >= row.rate_limit_1d * 0.8 ? 'bg-yellow-500' :
                      'bg-emerald-500'
                    ]"
                    :style="{ width: Math.min((row.usage_1d / row.rate_limit_1d) * 100, 100) + '%' }"
                  />
                </div>
                <div v-if="row.reset_1d_at && formatResetTime(row.reset_1d_at)" class="text-xs text-gray-400 dark:text-gray-500 tabular-nums">
                  ⟳ {{ formatResetTime(row.reset_1d_at) }}
                </div>
              </div>
              <!-- 7d window -->
              <div v-if="row.rate_limit_7d > 0">
                <div class="flex items-center justify-between text-sm">
                  <span class="text-gray-500 dark:text-gray-400">7d</span>
                  <span :class="[
                    'font-medium tabular-nums',
                    row.usage_7d >= row.rate_limit_7d ? 'text-red-500' :
                    row.usage_7d >= row.rate_limit_7d * 0.8 ? 'text-yellow-500' :
                    'text-gray-700 dark:text-gray-300'
                  ]">
                    ${{ row.usage_7d?.toFixed(2) || '0.00' }}/${{ row.rate_limit_7d?.toFixed(2) }}
                  </span>
                </div>
                <div class="h-1 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-all',
                      row.usage_7d >= row.rate_limit_7d ? 'bg-red-500' :
                      row.usage_7d >= row.rate_limit_7d * 0.8 ? 'bg-yellow-500' :
                      'bg-emerald-500'
                    ]"
                    :style="{ width: Math.min((row.usage_7d / row.rate_limit_7d) * 100, 100) + '%' }"
                  />
                </div>
                <div v-if="row.reset_7d_at && formatResetTime(row.reset_7d_at)" class="text-xs text-gray-400 dark:text-gray-500 tabular-nums">
                  ⟳ {{ formatResetTime(row.reset_7d_at) }}
                </div>
              </div>
              <!-- Reset button -->
              <button
                v-if="row.usage_5h > 0 || row.usage_1d > 0 || row.usage_7d > 0"
                @click.stop="confirmResetRateLimitFromTable(row)"
                class="mt-0.5 inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-sm text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                :title="t('keys.resetRateLimitUsage')"
              >
                <Icon name="refresh" size="xs" />
                {{ t('keys.resetUsage') }}
              </button>
            </div>
            <span v-else class="text-base text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-status="{ value }">
            <span :class="[
              'badge max-w-[92px] text-center leading-tight !whitespace-normal',
              value === 'active' ? 'badge-success' :
              value === 'quota_exhausted' ? 'badge-warning' :
              value === 'expired' ? 'badge-danger' :
              'badge-gray'
            ]">
              {{ t('keys.status.' + value) }}
            </span>
          </template>

          <!-- Date above time: two short lines instead of one wide timestamp. -->
          <template #cell-created_at="{ value }">
            <div class="text-sm leading-5 tabular-nums text-gray-500 dark:text-dark-400">
              <div>{{ formatDatePart(value) }}</div>
              <div>{{ formatTimePart(value) }}</div>
            </div>
          </template>

          <template #header-actions>
            <span class="block text-right">{{ t('common.actions') }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center justify-end">
              <!-- Use Key -->
              <button
                :disabled="!isRevealable(row)"
                @click="openUseKeyModal(row)"
                class="rounded-lg p-1 text-gray-500 transition-colors hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20 dark:hover:text-green-400"
                :class="{ 'cursor-not-allowed opacity-40 hover:bg-transparent hover:text-gray-500': !isRevealable(row) }"
                :title="!isRevealable(row) ? t('keys.keyNotRecoverable') : t('keys.useKey')"
                :aria-label="t('keys.useKey')"
              >
                <Icon name="terminal" size="sm" />
              </button>
              <!-- Import to CC Switch (admin only) -->
              <button
                v-if="isAdmin && !publicSettings?.hide_ccs_import_button"
                :disabled="!isRevealable(row)"
                @click="importToCcswitch(row)"
                class="rounded-lg p-1 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
                :class="{ 'cursor-not-allowed opacity-40 hover:bg-transparent hover:text-gray-500': !isRevealable(row) }"
                :title="!isRevealable(row) ? t('keys.keyNotRecoverable') : t('keys.importToCcSwitch')"
                :aria-label="t('keys.importToCcSwitch')"
              >
                <Icon name="upload" size="sm" />
              </button>
              <!-- Enable / Disable -->
              <button
                @click="toggleKeyStatus(row)"
                :class="[
                  'rounded-lg p-1 transition-colors',
                  row.status === 'active'
                    ? 'text-gray-500 hover:bg-yellow-50 hover:text-yellow-600 dark:hover:bg-yellow-900/20 dark:hover:text-yellow-400'
                    : 'text-gray-500 hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20 dark:hover:text-green-400'
                ]"
                :title="row.status === 'active' ? t('keys.disable') : t('keys.enable')"
                :aria-label="row.status === 'active' ? t('keys.disable') : t('keys.enable')"
              >
                <Icon v-if="row.status === 'active'" name="ban" size="sm" />
                <Icon v-else name="checkCircle" size="sm" />
              </button>
              <!-- Edit -->
              <button
                @click="editKey(row)"
                class="rounded-lg p-1 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                :title="t('common.edit')"
                :aria-label="t('common.edit')"
              >
                <Icon name="edit" size="sm" />
              </button>
              <!-- Delete -->
              <button
                @click="confirmDelete(row)"
                class="rounded-lg p-1 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                :title="t('common.delete')"
                :aria-label="t('common.delete')"
              >
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('keys.noKeysYet')"
              :description="t('keys.createFirstKey')"
              :action-text="t('keys.createKey')"
              @action="handleCreateKeyClick"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <!-- Create/Edit Modal -->
    <BaseDialog
      :show="showCreateModal || showEditModal"
      :title="showEditModal ? t('keys.editKey') : t('keys.createKey')"
      width="normal"
      @close="closeModals"
    >
      <form id="key-form" @submit.prevent="handleSubmit" class="space-y-5">
        <div v-if="isAdmin && !showEditModal">
          <label class="input-label">User / Owner</label>
          <select
            v-model="formData.user_id"
            class="input select w-full"
            required
          >
            <option :value="null" disabled>Select user...</option>
            <option
              v-for="user in allUsers"
              :key="user.id"
              :value="user.id"
            >
              {{ user.email }} ({{ user.username || 'no username' }})
            </option>
          </select>
        </div>

        <div>
          <label class="input-label">{{ t('keys.nameLabel') }}</label>
          <input
            v-model="formData.name"
            type="text"
            required
            class="input"
            :placeholder="t('keys.namePlaceholder')"
            data-tour="key-form-name"
          />
        </div>

        <!-- Batch creation: how many keys to mint at once (a random suffix is
             appended to the name when more than one is requested). -->
        <div v-if="!showEditModal">
          <label class="input-label">{{ t('keys.newQuantity') }}</label>
          <input
            v-model.number="formData.quantity"
            type="number"
            min="1"
            max="99"
            step="1"
            class="input"
            data-tour="key-form-quantity"
          />
          <p class="input-hint">{{ t('keys.newQuantityHint') }}</p>
        </div>

        <template v-if="isAdmin">
        <div>
          <label class="input-label">{{ t('keys.groupLabel') }}</label>
          <div v-if="formData.group_ids.length > 0" class="mb-2 flex flex-wrap gap-1.5">
            <GroupBadge
              v-for="group in groups.filter((item) => formData.group_ids.includes(item.id))"
              :key="group.id"
              :name="group.name"
              :platform="group.platform"
              :subscription-type="group.subscription_type"
              :rate-multiplier="group.rate_multiplier"
              :user-rate-multiplier="userGroupRates[group.id]"
            />
          </div>
          <div class="grid max-h-44 grid-cols-1 gap-1 overflow-y-auto rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-600 dark:bg-dark-800" data-tour="key-form-group">
            <label
              v-for="group in groups"
              :key="group.id"
              class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 transition-colors hover:bg-white dark:hover:bg-dark-700"
            >
              <input
                type="checkbox"
                :checked="formData.group_ids.includes(group.id)"
                @change="toggleFormGroup(group.id, ($event.target as HTMLInputElement).checked)"
                class="h-3.5 w-3.5 shrink-0 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
              />
              <GroupBadge
                :name="group.name"
                :platform="group.platform"
                :subscription-type="group.subscription_type"
                :rate-multiplier="group.rate_multiplier"
                :user-rate-multiplier="userGroupRates[group.id]"
                class="min-w-0 flex-1"
              />
            </label>
          </div>
        </div>
        </template>

        <!-- Model allowlist: which models this key may call (empty = all). -->
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('keys.modelAllowlist') }}</label>
              <p class="mt-1 text-xs text-gray-500">
                {{ t('keys.modelAllowlistHint') }}
              </p>
            </div>
            <button
              type="button"
              @click="formData.enable_model_restriction = !formData.enable_model_restriction"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                formData.enable_model_restriction ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  formData.enable_model_restriction ? 'translate-x-4' : 'translate-x-0'
                ]"
              />
            </button>
          </div>
          <div v-if="formData.enable_model_restriction" class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800">
            <ModelWhitelistSelector
              v-model="formData.allowed_models"
              :platforms="selectedGroupPlatforms"
            />
            <p class="mt-2 text-xs text-gray-500">
              {{
                formData.allowed_models.length > 0
                  ? t('admin.accounts.selectedModels', { count: formData.allowed_models.length })
                  : t('admin.accounts.supportsAllModels')
              }}
            </p>
          </div>
        </div>

        <!-- Custom Key Section (only for create, admin only) -->
        <template v-if="isAdmin">
        <div v-if="!showEditModal" class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.customKeyLabel') }}</label>
            <button
              type="button"
              @click="formData.use_custom_key = !formData.use_custom_key"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                formData.use_custom_key ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  formData.use_custom_key ? 'translate-x-4' : 'translate-x-0'
                ]"
              />
            </button>
          </div>
          <div v-if="formData.use_custom_key">
            <input
              v-model="formData.custom_key"
              type="text"
              class="input font-mono"
              :placeholder="t('keys.customKeyPlaceholder')"
              :class="{ 'border-red-500 dark:border-red-500': customKeyError }"
            />
            <p v-if="customKeyError" class="mt-1 text-sm text-red-500">{{ customKeyError }}</p>
            <p v-else class="input-hint">{{ t('keys.customKeyHint') }}</p>
          </div>
        </div>
        </template>

        <div v-if="showEditModal && isAdmin">
          <label class="input-label">{{ t('keys.statusLabel') }}</label>
          <Select
            v-model="formData.status"
            :options="statusOptions"
            :placeholder="t('keys.selectStatus')"
          />
        </div>

        <!-- IP Restriction Section -->
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.ipRestriction') }}</label>
            <button
              type="button"
              @click="formData.enable_ip_restriction = !formData.enable_ip_restriction"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                formData.enable_ip_restriction ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  formData.enable_ip_restriction ? 'translate-x-4' : 'translate-x-0'
                ]"
              />
            </button>
          </div>

          <div v-if="formData.enable_ip_restriction" class="space-y-4 pt-2">
            <div>
              <label class="input-label">{{ t('keys.ipWhitelist') }}</label>
              <textarea
                v-model="formData.ip_whitelist"
                rows="3"
                class="input font-mono text-sm"
                :placeholder="t('keys.ipWhitelistPlaceholder')"
              />
              <p class="input-hint">{{ t('keys.ipWhitelistHint') }}</p>
            </div>

            <div>
              <label class="input-label">{{ t('keys.ipBlacklist') }}</label>
              <textarea
                v-model="formData.ip_blacklist"
                rows="3"
                class="input font-mono text-sm"
                :placeholder="t('keys.ipBlacklistPlaceholder')"
              />
              <p class="input-hint">{{ t('keys.ipBlacklistHint') }}</p>
            </div>
          </div>
        </div>

        <!-- Quota Settings: unlimited (default) or a hard USD cap. -->
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('keys.quotaLimit') }}</label>
              <p class="mt-1 text-xs text-gray-500">{{ t('keys.quotaSettingsHint') }}</p>
            </div>
            <div class="flex items-center gap-2">
              <span class="text-xs text-gray-500">{{ t('keys.unlimitedQuota') }}</span>
              <button
                type="button"
                @click="formData.unlimited_quota = !formData.unlimited_quota"
                :class="[
                  'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                  formData.unlimited_quota ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
                ]"
              >
                <span
                  :class="[
                    'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                    formData.unlimited_quota ? 'translate-x-4' : 'translate-x-0'
                  ]"
                />
              </button>
            </div>
          </div>

          <div class="space-y-4">
            <div>
              <div class="relative">
                <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">$</span>
                <input
                  v-model.number="formData.quota"
                  type="number"
                  step="0.01"
                  min="0"
                  class="input pl-7"
                  :class="{ 'opacity-50': formData.unlimited_quota }"
                  :disabled="formData.unlimited_quota"
                  :placeholder="t('keys.quotaAmountPlaceholder')"
                />
              </div>
              <p class="input-hint">
                {{ formData.unlimited_quota ? t('keys.unlimitedQuotaHint') : t('keys.quotaAmountHint') }}
              </p>
            </div>

            <!-- Quota used display (only in edit mode) -->
            <div v-if="showEditModal && selectedKey && selectedKey.quota > 0">
              <label class="input-label">{{ t('keys.quotaUsed') }}</label>
              <div class="flex items-center gap-2">
                <div class="flex-1 rounded-lg bg-gray-100 px-3 py-2 dark:bg-dark-700">
                  <span class="font-medium text-gray-900 dark:text-white">
                    ${{ selectedKey.quota_used?.toFixed(4) || '0.0000' }}
                  </span>
                  <span class="mx-2 text-gray-400">/</span>
                  <span class="text-gray-500 dark:text-gray-400">
                    ${{ selectedKey.quota?.toFixed(2) || '0.00' }}
                  </span>
                </div>
                <button
                  type="button"
                  @click="confirmResetQuota"
                  class="btn btn-secondary text-sm"
                  :title="t('keys.resetQuotaUsed')"
                >
                  {{ t('keys.reset') }}
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- Rate Limit Section -->
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.rateLimitSection') }}</label>
            <button
              type="button"
              @click="formData.enable_rate_limit = !formData.enable_rate_limit"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                formData.enable_rate_limit ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  formData.enable_rate_limit ? 'translate-x-4' : 'translate-x-0'
                ]"
              />
            </button>
          </div>

          <div v-if="formData.enable_rate_limit" class="space-y-4 pt-2">
            <p class="input-hint -mt-2">{{ t('keys.rateLimitHint') }}</p>
            <!-- 5-Hour Limit -->
            <div>
              <label class="input-label">{{ t('keys.rateLimit5h') }}</label>
              <div class="relative">
                <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">$</span>
                <input
                  v-model.number="formData.rate_limit_5h"
                  type="number"
                  step="0.01"
                  min="0"
                  class="input pl-7"
                  :placeholder="'0'"
                />
              </div>
              <!-- Usage info (edit mode only) -->
              <div v-if="showEditModal && selectedKey && selectedKey.rate_limit_5h > 0" class="mt-2">
                <div class="flex items-center gap-2">
                  <div class="flex-1 rounded-lg bg-gray-100 px-3 py-2 dark:bg-dark-700 text-sm">
                    <span :class="[
                      'font-medium',
                      selectedKey.usage_5h >= selectedKey.rate_limit_5h ? 'text-red-500' :
                      selectedKey.usage_5h >= selectedKey.rate_limit_5h * 0.8 ? 'text-yellow-500' :
                      'text-gray-900 dark:text-white'
                    ]">
                      ${{ selectedKey.usage_5h?.toFixed(4) || '0.0000' }}
                    </span>
                    <span class="mx-2 text-gray-400">/</span>
                    <span class="text-gray-500 dark:text-gray-400">
                      ${{ selectedKey.rate_limit_5h?.toFixed(2) || '0.00' }}
                    </span>
                  </div>
                </div>
                <div class="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-all',
                      selectedKey.usage_5h >= selectedKey.rate_limit_5h ? 'bg-red-500' :
                      selectedKey.usage_5h >= selectedKey.rate_limit_5h * 0.8 ? 'bg-yellow-500' :
                      'bg-green-500'
                    ]"
                    :style="{ width: Math.min((selectedKey.usage_5h / selectedKey.rate_limit_5h) * 100, 100) + '%' }"
                  />
                </div>
              </div>
            </div>

            <!-- Daily Limit -->
            <div>
              <label class="input-label">{{ t('keys.rateLimit1d') }}</label>
              <div class="relative">
                <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">$</span>
                <input
                  v-model.number="formData.rate_limit_1d"
                  type="number"
                  step="0.01"
                  min="0"
                  class="input pl-7"
                  :placeholder="'0'"
                />
              </div>
              <!-- Usage info (edit mode only) -->
              <div v-if="showEditModal && selectedKey && selectedKey.rate_limit_1d > 0" class="mt-2">
                <div class="flex items-center gap-2">
                  <div class="flex-1 rounded-lg bg-gray-100 px-3 py-2 dark:bg-dark-700 text-sm">
                    <span :class="[
                      'font-medium',
                      selectedKey.usage_1d >= selectedKey.rate_limit_1d ? 'text-red-500' :
                      selectedKey.usage_1d >= selectedKey.rate_limit_1d * 0.8 ? 'text-yellow-500' :
                      'text-gray-900 dark:text-white'
                    ]">
                      ${{ selectedKey.usage_1d?.toFixed(4) || '0.0000' }}
                    </span>
                    <span class="mx-2 text-gray-400">/</span>
                    <span class="text-gray-500 dark:text-gray-400">
                      ${{ selectedKey.rate_limit_1d?.toFixed(2) || '0.00' }}
                    </span>
                  </div>
                </div>
                <div class="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-all',
                      selectedKey.usage_1d >= selectedKey.rate_limit_1d ? 'bg-red-500' :
                      selectedKey.usage_1d >= selectedKey.rate_limit_1d * 0.8 ? 'bg-yellow-500' :
                      'bg-green-500'
                    ]"
                    :style="{ width: Math.min((selectedKey.usage_1d / selectedKey.rate_limit_1d) * 100, 100) + '%' }"
                  />
                </div>
              </div>
            </div>

            <!-- 7-Day Limit -->
            <div>
              <label class="input-label">{{ t('keys.rateLimit7d') }}</label>
              <div class="relative">
                <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">$</span>
                <input
                  v-model.number="formData.rate_limit_7d"
                  type="number"
                  step="0.01"
                  min="0"
                  class="input pl-7"
                  :placeholder="'0'"
                />
              </div>
              <!-- Usage info (edit mode only) -->
              <div v-if="showEditModal && selectedKey && selectedKey.rate_limit_7d > 0" class="mt-2">
                <div class="flex items-center gap-2">
                  <div class="flex-1 rounded-lg bg-gray-100 px-3 py-2 dark:bg-dark-700 text-sm">
                    <span :class="[
                      'font-medium',
                      selectedKey.usage_7d >= selectedKey.rate_limit_7d ? 'text-red-500' :
                      selectedKey.usage_7d >= selectedKey.rate_limit_7d * 0.8 ? 'text-yellow-500' :
                      'text-gray-900 dark:text-white'
                    ]">
                      ${{ selectedKey.usage_7d?.toFixed(4) || '0.0000' }}
                    </span>
                    <span class="mx-2 text-gray-400">/</span>
                    <span class="text-gray-500 dark:text-gray-400">
                      ${{ selectedKey.rate_limit_7d?.toFixed(2) || '0.00' }}
                    </span>
                  </div>
                </div>
                <div class="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-all',
                      selectedKey.usage_7d >= selectedKey.rate_limit_7d ? 'bg-red-500' :
                      selectedKey.usage_7d >= selectedKey.rate_limit_7d * 0.8 ? 'bg-yellow-500' :
                      'bg-green-500'
                    ]"
                    :style="{ width: Math.min((selectedKey.usage_7d / selectedKey.rate_limit_7d) * 100, 100) + '%' }"
                  />
                </div>
              </div>
            </div>

            <!-- Reset Rate Limit button (edit mode only) -->
            <div v-if="showEditModal && selectedKey && (selectedKey.rate_limit_5h > 0 || selectedKey.rate_limit_1d > 0 || selectedKey.rate_limit_7d > 0)">
              <button
                type="button"
                @click="confirmResetRateLimit"
                class="btn btn-secondary text-sm"
              >
                {{ t('keys.resetRateLimitUsage') }}
              </button>
            </div>
          </div>
        </div>

        <!-- Expiration Section -->
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.expiration') }}</label>
            <div class="flex items-center gap-2">
              <span class="text-xs text-gray-500">
                {{ formData.enable_expiration ? t('keys.expiresIn') : t('keys.neverExpires') }}
              </span>
              <button
                type="button"
                @click="formData.enable_expiration = !formData.enable_expiration"
              :class="[
                'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                formData.enable_expiration ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  formData.enable_expiration ? 'translate-x-4' : 'translate-x-0'
                ]"
              />
            </button>
            </div>
          </div>

          <div v-if="formData.enable_expiration" class="space-y-4 pt-2">
            <!-- Quick select buttons (for both create and edit mode) -->
            <div class="flex flex-wrap gap-2">
              <button
                v-for="days in ['7', '30', '90']"
                :key="days"
                type="button"
                @click="setExpirationDays(parseInt(days))"
                :class="[
                  'rounded-lg px-3 py-1.5 text-sm transition-colors',
                  formData.expiration_preset === days
                    ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600'
                ]"
              >
                {{ showEditModal ? t('keys.extendDays', { days }) : t('keys.expiresInDays', { days }) }}
              </button>
              <button
                type="button"
                @click="formData.expiration_preset = 'custom'"
                :class="[
                  'rounded-lg px-3 py-1.5 text-sm transition-colors',
                  formData.expiration_preset === 'custom'
                    ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600'
                ]"
              >
                {{ t('keys.customDate') }}
              </button>
            </div>

            <!-- Date picker (always show for precise adjustment) -->
            <div>
              <label class="input-label">{{ t('keys.expirationDate') }}</label>
              <input
                v-model="formData.expiration_date"
                type="datetime-local"
                class="input"
              />
              <p class="input-hint">{{ t('keys.expirationDateHint') }}</p>
            </div>

            <!-- Current expiration display (only in edit mode) -->
            <div v-if="showEditModal && selectedKey?.expires_at" class="text-sm">
              <span class="text-gray-500 dark:text-gray-400">{{ t('keys.currentExpiration') }}: </span>
              <span class="font-medium text-gray-900 dark:text-white">
                {{ formatDateTime(selectedKey.expires_at) }}
              </span>
            </div>
          </div>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeModals" type="button" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button
            form="key-form"
            type="submit"
            :disabled="submitting"
            class="btn btn-primary"
            data-tour="key-form-submit"
          >
            <svg
              v-if="submitting"
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
            {{
              submitting
                ? t('keys.saving')
                : showEditModal
                  ? t('common.update')
                  : t('common.create')
            }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="createdKeys.length > 0"
      :title="t('keys.keyCreatedTitle')"
      width="normal"
      @close="closeCreatedKeyDialog"
    >
      <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('keys.keyCreatedWarning') }}</p>
      <div class="mt-4 space-y-2">
        <div
          v-for="(key, index) in createdKeys"
          :key="index"
          class="flex items-center gap-2 rounded-lg border border-amber-300 bg-amber-50 p-3 dark:border-amber-700 dark:bg-amber-900/20"
        >
          <code class="min-w-0 flex-1 break-all text-sm text-gray-900 dark:text-white">
            {{ revealedCreated[index] ? key : maskKey(key) }}
          </code>
          <button
            type="button"
            class="shrink-0 text-gray-500 hover:text-gray-700 dark:hover:text-gray-200"
            :title="revealedCreated[index] ? t('keys.hideKey') : t('keys.revealKey')"
            @click="toggleCreatedReveal(index)"
          >
            <Icon :name="revealedCreated[index] ? 'eyeOff' : 'eye'" size="sm" />
          </button>
          <button
            type="button"
            class="btn btn-secondary shrink-0"
            @click="copyCreatedKey(key)"
          >
            <Icon name="clipboard" size="sm" class="mr-1" />
            {{ t('keys.copyKey') }}
          </button>
        </div>
      </div>

      <QuickStart
        v-if="createdKeys.length > 0"
        :api-key="createdKeys[0]"
        :more-keys="createdKeys.length - 1"
        class="mt-6 border-t border-gray-100 pt-4 dark:border-dark-700"
      />
      <template #footer>
        <button v-if="createdKeys.length > 1" type="button" class="btn btn-secondary" @click="copyAllCreatedKeys">
          <Icon name="clipboard" size="sm" class="mr-1" />
          {{ t('keys.copyAllKeys') }}
        </button>
        <button type="button" class="btn btn-primary" @click="closeCreatedKeyDialog">
          {{ t('common.close') }}
        </button>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('keys.deleteKey')"
      :message="t('keys.deleteConfirmMessage', { name: selectedKey?.name })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="handleDelete"
      @cancel="showDeleteDialog = false"
    />

    <!-- Reset Quota Confirmation Dialog -->
    <ConfirmDialog
      :show="showResetQuotaDialog"
      :title="t('keys.resetQuotaTitle')"
      :message="t('keys.resetQuotaConfirmMessage', { name: selectedKey?.name, used: selectedKey?.quota_used?.toFixed(4) })"
      :confirm-text="t('keys.reset')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="resetQuotaUsed"
      @cancel="showResetQuotaDialog = false"
    />

    <!-- Reset Rate Limit Confirmation Dialog -->
    <ConfirmDialog
      :show="showResetRateLimitDialog"
      :title="t('keys.resetRateLimitTitle')"
      :message="t('keys.resetRateLimitConfirmMessage', { name: selectedKey?.name })"
      :confirm-text="t('keys.reset')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="resetRateLimitUsage"
      @cancel="showResetRateLimitDialog = false"
    />

    <!-- Use Key Modal -->
    <UseKeyModal
      :show="showUseKeyModal"
      :api-key="selectedKey?.key || ''"
      :base-url="publicSettings?.api_base_url || ''"
      :platform="selectedKey?.group?.platform || null"
      :allow-messages-dispatch="selectedKey?.group?.allow_messages_dispatch || false"
      @close="closeUseKeyModal"
    />

    <!-- CCS Client Selection Dialog for Antigravity -->
    <BaseDialog
      :show="showCcsClientSelect"
      :title="t('keys.ccsClientSelect.title')"
      width="narrow"
      @close="closeCcsClientSelect"
    >
      <div class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-gray-400">
          {{ t('keys.ccsClientSelect.description') }}
	        </p>
	        <div class="grid grid-cols-2 gap-3">
	          <button
	            @click="handleCcsClientSelect('claude')"
	            class="flex flex-col items-center gap-2 p-4 rounded-xl border-2 border-gray-200 dark:border-dark-600 hover:border-primary-500 dark:hover:border-primary-500 hover:bg-primary-50 dark:hover:bg-primary-900/20 transition-all"
	          >
	            <Icon name="terminal" size="xl" class="text-gray-600 dark:text-gray-400" />
	            <span class="font-medium text-gray-900 dark:text-white">{{
	              t('keys.ccsClientSelect.claudeCode')
	            }}</span>
	            <span class="text-xs text-gray-500 dark:text-gray-400">{{
	              t('keys.ccsClientSelect.claudeCodeDesc')
	            }}</span>
	          </button>
	          <button
	            @click="handleCcsClientSelect('gemini')"
	            class="flex flex-col items-center gap-2 p-4 rounded-xl border-2 border-gray-200 dark:border-dark-600 hover:border-primary-500 dark:hover:border-primary-500 hover:bg-primary-50 dark:hover:bg-primary-900/20 transition-all"
	          >
	            <Icon name="sparkles" size="xl" class="text-gray-600 dark:text-gray-400" />
	            <span class="font-medium text-gray-900 dark:text-white">{{
	              t('keys.ccsClientSelect.geminiCli')
	            }}</span>
	            <span class="text-xs text-gray-500 dark:text-gray-400">{{
	              t('keys.ccsClientSelect.geminiCliDesc')
	            }}</span>
	          </button>
	        </div>
	      </div>
      <template #footer>
        <div class="flex justify-end">
          <button @click="closeCcsClientSelect" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Group Selector Dropdown (Teleported to body to avoid overflow clipping) -->
    <Teleport to="body">
      <div
        v-if="groupSelectorKeyId !== null && dropdownPosition"
        ref="dropdownRef"
        class="animate-in fade-in slide-in-from-top-2 fixed z-[100000020] w-max min-w-[380px] overflow-hidden rounded-xl bg-white shadow-lg ring-1 ring-black/5 duration-200 dark:bg-dark-800 dark:ring-white/10"
        style="pointer-events: auto !important;"
        :style="{
          top: dropdownPosition.top !== undefined ? dropdownPosition.top + 'px' : undefined,
          bottom: dropdownPosition.bottom !== undefined ? dropdownPosition.bottom + 'px' : undefined,
          left: dropdownPosition.left + 'px'
        }"
      >
        <!-- Search box -->
        <div class="border-b border-gray-100 p-2 dark:border-dark-700">
          <div class="relative">
            <svg class="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            <input
              v-model="groupSearchQuery"
              type="text"
              class="w-full rounded-lg border border-gray-200 bg-gray-50 py-1.5 pl-8 pr-3 text-sm text-gray-900 placeholder-gray-400 outline-none focus:border-primary-300 focus:ring-1 focus:ring-primary-300 dark:border-dark-600 dark:bg-dark-700 dark:text-white dark:placeholder-gray-500 dark:focus:border-primary-600 dark:focus:ring-primary-600"
              :placeholder="t('keys.searchGroup')"
              @click.stop
            />
          </div>
        </div>
        <!-- Group list -->
        <div class="max-h-80 overflow-y-auto p-1.5">
          <button
            v-for="option in filteredGroupOptions"
            :key="option.value ?? 'null'"
            @click="changeGroup(selectedKeyForGroup!, option.value)"
            :class="[
              'flex w-full items-center justify-between rounded-lg px-3 py-2.5 text-sm transition-colors',
              'border-b border-gray-100 last:border-0 dark:border-dark-700',
              selectedKeyForGroup?.group_id === option.value ||
              (!selectedKeyForGroup?.group_id && option.value === null)
                ? 'bg-primary-50 dark:bg-primary-900/20'
                : 'hover:bg-gray-100 dark:hover:bg-dark-700'
            ]"
            :title="option.description || undefined"
          >
            <GroupOptionItem
              :name="option.label"
              :platform="option.platform"
              :subscription-type="option.subscriptionType"
              :rate-multiplier="option.rate"
              :user-rate-multiplier="option.userRate"
              :description="option.description"
              :selected="
                selectedKeyForGroup?.group_id === option.value ||
                (!selectedKeyForGroup?.group_id && option.value === null)
              "
            />
          </button>
          <!-- Empty state when search has no results -->
          <div v-if="filteredGroupOptions.length === 0" class="py-4 text-center text-sm text-gray-400 dark:text-gray-500">
            {{ t('keys.noGroupFound') }}
          </div>
        </div>
      </div>
    </Teleport>
  </AppLayout>
</template>

<script setup lang="ts">
	import { ref, computed, watch, onMounted, onUnmounted, type ComponentPublicInstance } from 'vue'
	import { useI18n } from 'vue-i18n'
	import { useAppStore } from '@/stores/app'
	import { useOnboardingStore } from '@/stores/onboarding'
	import { useClipboard } from '@/composables/useClipboard'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'

const { t } = useI18n()
import { keysAPI, authAPI, usageAPI, userGroupsAPI } from '@/api'
import { adminAPI } from '@/api/admin'
import { useAuthStore } from '@/stores/auth'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import QuickStart from '@/components/keys/QuickStart.vue'
	import DataTable from '@/components/common/DataTable.vue'
	import Pagination from '@/components/common/Pagination.vue'
	import BaseDialog from '@/components/common/BaseDialog.vue'
	import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
	import EmptyState from '@/components/common/EmptyState.vue'
	import Select from '@/components/common/Select.vue'
	import SearchInput from '@/components/common/SearchInput.vue'
	import Icon from '@/components/icons/Icon.vue'
	import UseKeyModal from '@/components/keys/UseKeyModal.vue'
	import GroupBadge from '@/components/common/GroupBadge.vue'
	import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
	import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
	import type { ApiKey, Group, PublicSettings } from '@/types'
import type { Column } from '@/components/common/types'
import type { BatchApiKeyUsageStats } from '@/api/usage'
import { formatDateTime } from '@/utils/format'
import { getPublicApiBaseUrl } from '@/utils/url'

// Helper to format date for datetime-local input
const formatDateTimeLocal = (isoDate: string): string => {
  const date = new Date(isoDate)
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

// The "Created" cell renders the date and the time on two lines to keep the
// column narrow; both parts come from the same formatted timestamp.
const formatDatePart = (value: string): string => (formatDateTime(value) || '').split(' ')[0] || ''
const formatTimePart = (value: string): string => (formatDateTime(value) || '').split(' ')[1] || ''

const appStore = useAppStore()
const onboardingStore = useOnboardingStore()
const { copyToClipboard: clipboardCopy } = useClipboard()

// Column set follows the OpenLux provider console: name, status, masked key
// (with copy), limit, usage, created, actions. The admin additionally keeps the
// owner, group and rate-limit columns.
//
// Width rule: each column gets "what its content needs + an equal share of the
// leftover". `100%` inside the calc resolves against the table (which is
// `width: 100%`), so the shares add back up to exactly the full width, and
// `table-layout: auto` still lets a column grow past its share when its content
// genuinely needs more (nothing gets clipped, and the no-horizontal-scroll
// guarantee at 1440px holds).
//
// The classes are written out literally on purpose: Tailwind scans the source
// text, so a class assembled from a template literal would never be generated.
// Totals: the user table needs 920px of content across 7 columns, the admin
// table 1240px across 10.
const USER_COL = {
  name: 'w-[calc(220px_+_(100%_-_1090px)_/_7)]',
  status: 'w-[calc(120px_+_(100%_-_1090px)_/_7)]',
  key: 'w-[calc(250px_+_(100%_-_1090px)_/_7)]',
  limit: 'w-[calc(110px_+_(100%_-_1090px)_/_7)]',
  usage: 'w-[calc(110px_+_(100%_-_1090px)_/_7)]',
  created: 'w-[calc(130px_+_(100%_-_1090px)_/_7)]',
  actions: 'w-[calc(150px_+_(100%_-_1090px)_/_7)]'
}

const ADMIN_COL = {
  name: 'w-[calc(220px_+_(100%_-_1470px)_/_10)]',
  status: 'w-[calc(120px_+_(100%_-_1470px)_/_10)]',
  key: 'w-[calc(250px_+_(100%_-_1470px)_/_10)]',
  limit: 'w-[calc(110px_+_(100%_-_1470px)_/_10)]',
  usage: 'w-[calc(110px_+_(100%_-_1470px)_/_10)]',
  owner: 'w-[calc(120px_+_(100%_-_1470px)_/_10)]',
  group: 'w-[calc(130px_+_(100%_-_1470px)_/_10)]',
  rate_limit: 'w-[calc(130px_+_(100%_-_1470px)_/_10)]',
  created: 'w-[calc(130px_+_(100%_-_1470px)_/_10)]',
  actions: 'w-[calc(150px_+_(100%_-_1470px)_/_10)] text-right'
}

const columns = computed<Column[]>(() => {
  if (isAdmin.value) {
    return [
      { key: 'name', label: t('common.name'), sortable: true, class: ADMIN_COL.name },
      { key: 'status', label: t('common.status'), sortable: true, class: ADMIN_COL.status },
      { key: 'key', label: t('keys.apiKey'), sortable: false, class: ADMIN_COL.key },
      { key: 'limit', label: t('keys.limit'), sortable: false, class: ADMIN_COL.limit },
      { key: 'usage', label: t('keys.usage'), sortable: false, class: ADMIN_COL.usage },
      { key: 'owner', label: 'Owner', sortable: false, class: ADMIN_COL.owner },
      { key: 'group', label: t('keys.group'), sortable: false, class: ADMIN_COL.group },
      { key: 'rate_limit', label: t('keys.rateLimitColumn'), sortable: false, class: ADMIN_COL.rate_limit },
      { key: 'created_at', label: t('keys.created'), sortable: true, class: ADMIN_COL.created },
      { key: 'actions', label: t('common.actions'), sortable: false, class: ADMIN_COL.actions }
    ]
  }
  return [
    { key: 'name', label: t('common.name'), sortable: true, class: USER_COL.name },
    { key: 'status', label: t('common.status'), sortable: true, class: USER_COL.status },
    { key: 'key', label: t('keys.apiKey'), sortable: false, class: USER_COL.key },
    { key: 'limit', label: t('keys.limit'), sortable: false, class: USER_COL.limit },
    { key: 'usage', label: t('keys.usage'), sortable: false, class: USER_COL.usage },
    { key: 'created_at', label: t('keys.created'), sortable: true, class: USER_COL.created },
    { key: 'actions', label: t('common.actions'), sortable: false, class: USER_COL.actions }
  ]
})

const apiKeys = ref<ApiKey[]>([])
const groups = ref<Group[]>([])
const loading = ref(false)
const submitting = ref(false)


const now = ref(new Date())
let resetTimer: ReturnType<typeof setInterval> | null = null
const usageStats = ref<Record<string, BatchApiKeyUsageStats>>({})
const userGroupRates = ref<Record<number, number>>({})

const pagination = ref({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})

// Filter state
const filterSearch = ref('')
const filterStatus = ref('')
const filterGroupId = ref<string | number>('')
const filterUserId = ref<string | number>('')

const showCreateModal = ref(false)
const showEditModal = ref(false)
const showDeleteDialog = ref(false)
const showResetQuotaDialog = ref(false)
const showResetRateLimitDialog = ref(false)
const showUseKeyModal = ref(false)
const showCcsClientSelect = ref(false)
const createdKeys = ref<string[]>([])
// Whether each freshly created key is revealed in the "API Key Created" dialog.
const revealedCreated = ref<Record<number, boolean>>({})
const pendingCcsRow = ref<ApiKey | null>(null)
const selectedKey = ref<ApiKey | null>(null)
const copiedKeyId = ref<number | null>(null)
const groupSelectorKeyId = ref<number | null>(null)
const publicSettings = ref<PublicSettings | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const dropdownPosition = ref<{ top?: number; bottom?: number; left: number } | null>(null)
const groupButtonRefs = ref<Map<number, HTMLElement>>(new Map())
let abortController: AbortController | null = null

// Get the currently selected key for group change
const selectedKeyForGroup = computed(() => {
  if (groupSelectorKeyId.value === null) return null
  return apiKeys.value.find((k) => k.id === groupSelectorKeyId.value) || null
})

const setGroupButtonRef = (keyId: number, el: Element | ComponentPublicInstance | null) => {
  if (el instanceof HTMLElement) {
    groupButtonRefs.value.set(keyId, el)
  } else {
    groupButtonRefs.value.delete(keyId)
  }
}

const toggleFormGroup = (groupId: number, checked: boolean) => {
  if (checked) {
    if (!formData.value.group_ids.includes(groupId)) {
      formData.value.group_ids = [...formData.value.group_ids, groupId]
    }
    return
  }
  formData.value.group_ids = formData.value.group_ids.filter((id) => id !== groupId)
}

const formData = ref({
  user_id: null as number | null,
  name: '',
  group_ids: [] as number[],
  enable_model_restriction: false,
  allowed_models: [] as string[],
  status: 'active' as 'active' | 'inactive',
  use_custom_key: false,
  custom_key: '',
  enable_ip_restriction: false,
  ip_whitelist: '',
  ip_blacklist: '',
  // Quota settings: unlimited by default, or a hard USD cap.
  enable_quota: false,
  unlimited_quota: true,
  quota: null as number | null,
  // How many keys to mint on create (batch); >1 appends a random name suffix.
  quantity: 1,
  // Rate limit settings
  enable_rate_limit: false,
  rate_limit_5h: null as number | null,
  rate_limit_1d: null as number | null,
  rate_limit_7d: null as number | null,
  enable_expiration: false,
  expiration_preset: '30' as '7' | '30' | '90' | 'custom',
  expiration_date: ''
})

const authStore = useAuthStore()
const isAdmin = computed(() => authStore.isAdmin)
const allUsers = ref<any[]>([])

const loadAllUsers = async () => {
  if (!isAdmin.value) return
  try {
    const res = await adminAPI.users.list(1, 1000)
    allUsers.value = res.items || []
  } catch (err) {
    console.error('Failed to load users for dropdown:', err)
  }
}

watch(showCreateModal, (val) => {
  if (val && isAdmin.value) {
    loadAllUsers()
    formData.value.user_id = authStore.user?.id || null
  }
})

// 自定义Key验证
const customKeyError = computed(() => {
  if (!formData.value.use_custom_key || !formData.value.custom_key) {
    return ''
  }
  const key = formData.value.custom_key
  if (key.length < 16) {
    return t('keys.customKeyTooShort')
  }
  // 检查字符：只允许字母、数字、下划线、连字符
  if (!/^[a-zA-Z0-9_-]+$/.test(key)) {
    return t('keys.customKeyInvalidChars')
  }
  return ''
})

const statusOptions = computed(() => [
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') }
])

// Filter dropdown options
const groupFilterOptions = computed(() => [
  { value: '', label: t('keys.allGroups') },
  { value: 0, label: t('keys.noGroup') },
  ...groups.value.map((g) => ({ value: g.id, label: g.name }))
])

const statusFilterOptions = computed(() => [
  { value: '', label: t('keys.allStatus') },
  { value: 'active', label: t('keys.status.active') },
  { value: 'inactive', label: t('keys.status.inactive') },
  { value: 'quota_exhausted', label: t('keys.status.quota_exhausted') },
  { value: 'expired', label: t('keys.status.expired') }
])

const onFilterChange = () => {
  pagination.value.page = 1
  loadApiKeys()
}

const onGroupFilterChange = (value: string | number | boolean | null) => {
  filterGroupId.value = value as string | number
  onFilterChange()
}

const onUserFilterChange = (value: string | number | boolean | null) => {
  filterUserId.value = value as string | number
  onFilterChange()
}

const userFilterOptions = computed(() => {
  const options = [
    { label: t('common.allUsers') || 'All Users', value: '' }
  ]
  allUsers.value.forEach(u => {
    options.push({
      label: `${u.email} (${u.username || 'no name'})`,
      value: u.id
    })
  })
  return options
})

const onStatusFilterChange = (value: string | number | boolean | null) => {
  filterStatus.value = value as string
  onFilterChange()
}

// Convert groups to Select options format with rate multiplier and subscription type
const groupOptions = computed(() =>
  groups.value.map((group) => ({
    value: group.id,
    label: group.name,
    description: group.description,
    rate: group.rate_multiplier,
    userRate: userGroupRates.value[group.id] ?? null,
    subscriptionType: group.subscription_type,
    platform: group.platform
  }))
)

const selectedGroupPlatforms = computed(() => {
  const platforms = new Set<string>()
  for (const group of groups.value) {
    if (formData.value.group_ids.includes(group.id) && group.platform) {
      platforms.add(group.platform)
    }
  }
  return Array.from(platforms)
})

// Group dropdown search
const groupSearchQuery = ref('')
const filteredGroupOptions = computed(() => {
  const query = groupSearchQuery.value.trim().toLowerCase()
  if (!query) return groupOptions.value
  return groupOptions.value.filter((opt) => {
    return opt.label.toLowerCase().includes(query) ||
      (opt.description && opt.description.toLowerCase().includes(query))
  })
})

const maskKey = (key: string): string => {
  if (key.length <= 12) return key
  return `${key.slice(0, 8)}...${key.slice(-4)}`
}

// The keys list never returns a secret: it returns the AT-REST lookup token
// (`sha256:<first 8 characters of the key>:<hex digest>`, see
// `hashAPIKeyForStorage` in backend/internal/repository/api_key_repo.go). The
// retained prefix is exactly what a console mask should show — `sk-9f3a21c4…`.
const AT_REST_KEY_PATTERN = /^sha256:([^:]*):[0-9a-f]+$/i

/** `sk-9f3a21c4…` for an at-rest token; falls back to the generic mask. */
const maskStoredKey = (stored: string): string => {
  if (!stored) return ''
  const match = AT_REST_KEY_PATTERN.exec(stored)
  if (match) return `${match[1]}…`
  // Legacy rows (pre-migration 137) may still hold the plaintext value.
  return maskKey(stored)
}

// --- Recoverable (revealable) keys -----------------------------------------
// The full value is fetched on demand (encrypted at rest on the server) and
// cached for the session. Keys created before recoverable storage was enabled
// report has_full_key=false and stay irrecoverable.
const revealedKeys = ref<Record<number, string>>({})
const revealingKeyId = ref<number | null>(null)

const isRevealable = (row: ApiKey): boolean => !!row.has_full_key

/** Reveal a key's full value (cached). Returns null when not recoverable. */
const revealKey = async (row: ApiKey): Promise<string | null> => {
  const cached = revealedKeys.value[row.id]
  if (cached) return cached
  if (revealingKeyId.value === row.id) return null
  revealingKeyId.value = row.id
  try {
    const value = await keysAPI.reveal(row.id)
    revealedKeys.value = { ...revealedKeys.value, [row.id]: value }
    return value
  } catch (error) {
    console.error('Failed to reveal API key:', error)
    appStore.showError(t('keys.revealFailed'))
    return null
  } finally {
    revealingKeyId.value = null
  }
}

const displayKey = (row: ApiKey, stored: string): string =>
  revealedKeys.value[row.id] || maskStoredKey(stored)

const toggleReveal = async (row: ApiKey) => {
  if (revealedKeys.value[row.id]) {
    const next = { ...revealedKeys.value }
    delete next[row.id]
    revealedKeys.value = next
    return
  }
  await revealKey(row)
}

const copyFullKey = async (row: ApiKey) => {
  const value = await revealKey(row)
  if (value) await copyToClipboard(value, row.id)
}

const copyToClipboard = async (text: string, keyId: number) => {
  const success = await clipboardCopy(text, t('keys.copied'))
  if (success) {
    copiedKeyId.value = keyId
    setTimeout(() => {
      copiedKeyId.value = null
    }, 800)
  }
}

const copyCreatedKey = async (key: string) => {
  if (key) {
    await clipboardCopy(key, t('keys.copied'))
  }
}

const copyAllCreatedKeys = async () => {
  if (createdKeys.value.length > 0) {
    await clipboardCopy(createdKeys.value.join('\n'), t('keys.copied'))
  }
}

const closeCreatedKeyDialog = () => {
  createdKeys.value = []
  revealedCreated.value = {}
}

/** Show/hide one freshly created key in the "API Key Created" dialog. */
const toggleCreatedReveal = (index: number) => {
  revealedCreated.value = { ...revealedCreated.value, [index]: !revealedCreated.value[index] }
}

const isAbortError = (error: unknown) => {
  if (!error || typeof error !== 'object') return false
  const { name, code } = error as { name?: string; code?: string }
  return name === 'AbortError' || code === 'ERR_CANCELED'
}

const loadApiKeys = async () => {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  const { signal } = controller
  loading.value = true
  try {
    // Build filters
    const filters: { search?: string; status?: string; group_id?: number | string; user_id?: number | string } = {}
    if (filterSearch.value) filters.search = filterSearch.value
    if (filterStatus.value) filters.status = filterStatus.value
    if (filterGroupId.value !== '') filters.group_id = filterGroupId.value
    if (filterUserId.value !== '') filters.user_id = filterUserId.value

    const listFn = isAdmin.value ? keysAPI.listAdmin : keysAPI.list
    const response = await listFn(pagination.value.page, pagination.value.page_size, filters, {
      signal
    })
    if (signal.aborted) return
    apiKeys.value = response.items
    pagination.value.total = response.total
    pagination.value.pages = response.pages

    // Load usage stats for all API keys in the list
    if (response.items.length > 0) {
      const keyIds = response.items.map((k) => k.id)
      try {
        const usageResponse = await usageAPI.getDashboardApiKeysUsage(keyIds, { signal })
        if (signal.aborted) return
        // A response without `stats` must not leave this undefined: the table template
        // reads `usageStats[row.id]`, and an undefined map throws on EVERY row render —
        // which leaves the table empty ("Showing 1 to 1 of 1 results" with no rows) and
        // aborts the in-flight patch, so a dialog that was just told to close stays on screen.
        usageStats.value = usageResponse?.stats ?? {}
      } catch (e) {
        if (!isAbortError(e)) {
          console.error('Failed to load usage stats:', e)
        }
      }
    }
  } catch (error) {
    if (isAbortError(error)) {
      return
    }
    appStore.showError(t('keys.failedToLoad'))
  } finally {
    if (abortController === controller) {
      loading.value = false
    }
  }
}

const loadGroups = async () => {
  try {
    groups.value = await userGroupsAPI.getAvailable()
  } catch (error) {
    console.error('Failed to load groups:', error)
  }
}

const loadUserGroupRates = async () => {
  try {
    userGroupRates.value = await userGroupsAPI.getUserGroupRates()
  } catch (error) {
    console.error('Failed to load user group rates:', error)
  }
}

const loadPublicSettings = async () => {
  try {
    publicSettings.value = await authAPI.getPublicSettings()
  } catch (error) {
    console.error('Failed to load public settings:', error)
  }
}

const openUseKeyModal = async (key: ApiKey) => {
  // The snippet builder needs the real value, so reveal it first.
  const value = await revealKey(key)
  if (!value) return
  selectedKey.value = { ...key, key: value }
  showUseKeyModal.value = true
}

const closeUseKeyModal = () => {
  showUseKeyModal.value = false
  selectedKey.value = null
}

const handlePageChange = (page: number) => {
  pagination.value.page = page
  loadApiKeys()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.value.page_size = pageSize
  pagination.value.page = 1
  loadApiKeys()
}

const editKey = (key: ApiKey) => {
  selectedKey.value = key
  const hasIPRestriction = (key.ip_whitelist?.length > 0) || (key.ip_blacklist?.length > 0)
  const hasExpiration = !!key.expires_at
  formData.value = {
    user_id: key.user_id || null,
    name: key.name,
    group_ids: key.group_ids && key.group_ids.length > 0
      ? [...key.group_ids]
      : (key.group_id ? [key.group_id] : []),
    enable_model_restriction: (key.allowed_models?.length || 0) > 0,
    allowed_models: [...(key.allowed_models || [])],
    status: key.status === 'quota_exhausted' || key.status === 'expired' ? 'inactive' : key.status,
    use_custom_key: false,
    custom_key: '',
    enable_ip_restriction: hasIPRestriction,
    ip_whitelist: (key.ip_whitelist || []).join('\n'),
    ip_blacklist: (key.ip_blacklist || []).join('\n'),
    enable_quota: key.quota > 0,
    unlimited_quota: !(key.quota > 0),
    quantity: 1,
    quota: key.quota > 0 ? key.quota : null,
    enable_rate_limit: (key.rate_limit_5h > 0) || (key.rate_limit_1d > 0) || (key.rate_limit_7d > 0),
    rate_limit_5h: key.rate_limit_5h || null,
    rate_limit_1d: key.rate_limit_1d || null,
    rate_limit_7d: key.rate_limit_7d || null,
    enable_expiration: hasExpiration,
    expiration_preset: 'custom',
    expiration_date: key.expires_at ? formatDateTimeLocal(key.expires_at) : ''
  }
  showEditModal.value = true
}

const toggleKeyStatus = async (key: ApiKey) => {
  const newStatus = key.status === 'active' ? 'inactive' : 'active'
  try {
    if (isAdmin.value) {
      await keysAPI.updateAdmin(key.id, { status: newStatus })
    } else {
      await keysAPI.toggleStatus(key.id, newStatus)
    }
    appStore.showSuccess(
      newStatus === 'active' ? t('keys.keyEnabledSuccess') : t('keys.keyDisabledSuccess')
    )
    loadApiKeys()
  } catch (error) {
    appStore.showError(t('keys.failedToUpdateStatus'))
  }
}

const changeGroup = async (key: ApiKey, newGroupId: number | null) => {
  groupSelectorKeyId.value = null
  dropdownPosition.value = null
  if (key.group_id === newGroupId) return

  try {
    const updateFn = isAdmin.value ? keysAPI.updateAdmin : keysAPI.update
    await updateFn(key.id, { group_id: newGroupId, group_ids: newGroupId ? [newGroupId] : [] })
    appStore.showSuccess(t('keys.groupChangedSuccess'))
    loadApiKeys()
  } catch (error) {
    appStore.showError(t('keys.failedToChangeGroup'))
  }
}

const closeGroupSelector = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  // Check if click is inside the dropdown or the trigger button
  if (!target.closest('.group\\/dropdown') && !dropdownRef.value?.contains(target)) {
    groupSelectorKeyId.value = null
    dropdownPosition.value = null
  }
}

const confirmDelete = (key: ApiKey) => {
  selectedKey.value = key
  showDeleteDialog.value = true
}

const handleSubmit = async () => {
  if (!isAdmin.value) {
    await submitUserKey()
    return
  }

  if (formData.value.group_ids.length === 0) {
    appStore.showError(t('keys.groupRequired'))
    return
  }

  // Validate custom key if enabled
  if (!showEditModal.value && formData.value.use_custom_key) {
    if (!formData.value.custom_key) {
      appStore.showError(t('keys.customKeyRequired'))
      return
    }
    if (customKeyError.value) {
      appStore.showError(customKeyError.value)
      return
    }
  }

  // Parse IP lists only if IP restriction is enabled
  const parseIPList = (text: string): string[] =>
    text.split('\n').map(ip => ip.trim()).filter(ip => ip.length > 0)
  const ipWhitelist = formData.value.enable_ip_restriction ? parseIPList(formData.value.ip_whitelist) : []
  const ipBlacklist = formData.value.enable_ip_restriction ? parseIPList(formData.value.ip_blacklist) : []

  // Calculate quota value: the "unlimited" switch wins, otherwise a positive
  // amount is a hard cap (both stored as 0 = unlimited).
  const quota = formData.value.unlimited_quota
    ? 0
    : (formData.value.quota && formData.value.quota > 0 ? formData.value.quota : 0)
  const allowedModels = formData.value.enable_model_restriction
    ? Array.from(new Set(formData.value.allowed_models.map(model => model.trim()).filter(Boolean)))
    : []

  // Calculate expiration
  let expiresInDays: number | undefined
  let expiresAt: string | null | undefined
  if (formData.value.enable_expiration && formData.value.expiration_date) {
    if (!showEditModal.value) {
      // Create mode: calculate days from date
      const expDate = new Date(formData.value.expiration_date)
      const now = new Date()
      const diffDays = Math.ceil((expDate.getTime() - now.getTime()) / (1000 * 60 * 60 * 24))
      expiresInDays = diffDays > 0 ? diffDays : 1
    } else {
      // Edit mode: use custom date directly
      expiresAt = new Date(formData.value.expiration_date).toISOString()
    }
  } else if (showEditModal.value) {
    // Edit mode: if expiration disabled or date cleared, send empty string to clear
    expiresAt = ''
  }

  // Calculate rate limit values (send 0 when toggle is off)
  const rateLimitData = formData.value.enable_rate_limit ? {
    rate_limit_5h: formData.value.rate_limit_5h && formData.value.rate_limit_5h > 0 ? formData.value.rate_limit_5h : 0,
    rate_limit_1d: formData.value.rate_limit_1d && formData.value.rate_limit_1d > 0 ? formData.value.rate_limit_1d : 0,
    rate_limit_7d: formData.value.rate_limit_7d && formData.value.rate_limit_7d > 0 ? formData.value.rate_limit_7d : 0,
  } : { rate_limit_5h: 0, rate_limit_1d: 0, rate_limit_7d: 0 }

  submitting.value = true
  try {
    if (showEditModal.value && selectedKey.value) {
      const updateFn = isAdmin.value ? keysAPI.updateAdmin : keysAPI.update
      await updateFn(selectedKey.value.id, {
        name: formData.value.name,
        group_id: formData.value.group_ids[0] ?? null,
        group_ids: formData.value.group_ids,
        allowed_models: allowedModels,
        status: formData.value.status,
        ip_whitelist: ipWhitelist,
        ip_blacklist: ipBlacklist,
        quota: quota,
        expires_at: expiresAt,
        rate_limit_5h: rateLimitData.rate_limit_5h,
        rate_limit_1d: rateLimitData.rate_limit_1d,
        rate_limit_7d: rateLimitData.rate_limit_7d,
      })
      appStore.showSuccess(t('keys.keyUpdatedSuccess'))
    } else {
      const customKey = formData.value.use_custom_key ? formData.value.custom_key : undefined
      const newKey = await keysAPI.create(
        formData.value.name,
        formData.value.group_ids[0] ?? null,
        formData.value.group_ids,
        allowedModels,
        customKey,
        ipWhitelist,
        ipBlacklist,
        quota,
        expiresInDays,
        rateLimitData,
        formData.value.user_id ?? undefined
      )
      createdKeys.value = [newKey.key]
      appStore.showSuccess(t('keys.keyCreatedSuccess'))
      // Only advance tour if active, on submit step, and creation succeeded
      if (onboardingStore.isCurrentStep('[data-tour="key-form-submit"]')) {
        onboardingStore.nextStep(500)
      }
    }
    closeModals()
    loadApiKeys()
  } catch (error: any) {
    const errorMsg = error.response?.data?.detail || t('keys.failedToSave')
    appStore.showError(errorMsg)
    // Don't advance tour on error
  } finally {
    submitting.value = false
  }
}

const handleCreateKeyClick = async () => {
  if (!isAdmin.value && (!groups.value || groups.value.length === 0)) {
    await loadGroups()
  }
  if (!showEditModal.value) {
    // Regular users do not pick a group: bind the default (yunwu) group up front
    // so the model allowlist can resolve the group's platforms.
    formData.value.group_ids = isAdmin.value ? [] : (defaultUserGroupId.value ? [defaultUserGroupId.value] : [])
  }
  showCreateModal.value = true
}

/** Short random suffix appended to each name when minting keys in batches. */
const randomSuffix = (): string => Math.random().toString(36).slice(2, 6)

// Default group used for regular-user key creation: the yunwu group when present.
const defaultUserGroupId = computed<number | null>(() => {
  const yunwuGroup =
    groups.value.find((g) => g.name.toLowerCase() === 'yunwu') ||
    groups.value.find((g) => g.name.toLowerCase().includes('yunwu')) ||
    groups.value[0]
  return yunwuGroup ? yunwuGroup.id : null
})

// Key management for regular users: label, expiration, batch quantity, quota
// (unlimited or a USD cap), model allowlist and IP allowlist. Admin-only fields
// (owner, groups, custom key, rate limits, status) are never sent from here.
const submitUserKey = async () => {
  const name = formData.value.name.trim()
  if (!name) {
    appStore.showError(t('keys.namePlaceholder'))
    return
  }

  const parseList = (text: string): string[] =>
    text.split('\n').map((value) => value.trim()).filter((value) => value.length > 0)
  const ipWhitelist = formData.value.enable_ip_restriction ? parseList(formData.value.ip_whitelist) : []
  const ipBlacklist = formData.value.enable_ip_restriction ? parseList(formData.value.ip_blacklist) : []
  const allowedModels = formData.value.enable_model_restriction
    ? Array.from(new Set(formData.value.allowed_models.map((model) => model.trim()).filter(Boolean)))
    : []
  const quota = formData.value.unlimited_quota
    ? 0
    : (formData.value.quota && formData.value.quota > 0 ? formData.value.quota : 0)
  let expiresInDays: number | undefined
  if (formData.value.enable_expiration && formData.value.expiration_date) {
    const diffDays = Math.ceil((new Date(formData.value.expiration_date).getTime() - Date.now()) / 86400000)
    expiresInDays = diffDays > 0 ? diffDays : 1
  }

  submitting.value = true
  try {
    if (showEditModal.value && selectedKey.value) {
      await keysAPI.update(selectedKey.value.id, {
        name,
        allowed_models: allowedModels,
        // The backend overwrites the IP lists unconditionally on update
        // (internal/service/api_key_service.go), so echo the existing values
        // back when the IP section was left untouched.
        ip_whitelist: formData.value.enable_ip_restriction ? ipWhitelist : (selectedKey.value.ip_whitelist || []),
        ip_blacklist: formData.value.enable_ip_restriction ? ipBlacklist : (selectedKey.value.ip_blacklist || []),
        quota,
        expires_at: formData.value.enable_expiration && formData.value.expiration_date
          ? new Date(formData.value.expiration_date).toISOString()
          : ''
      })
      appStore.showSuccess(t('keys.keyUpdatedSuccess'))
    } else {
      const gid = defaultUserGroupId.value
      // Batch creation: 1..99 keys, each name gets a random suffix when >1.
      const quantity = Math.min(Math.max(Math.trunc(Number(formData.value.quantity)) || 1, 1), 99)
      const created: string[] = []
      for (let index = 0; index < quantity; index += 1) {
        const keyName = quantity > 1 ? `${name}-${randomSuffix()}` : name
        const newKey = await keysAPI.create(
          keyName,
          gid,
          gid ? [gid] : [],
          allowedModels,
          undefined,
          ipWhitelist,
          ipBlacklist,
          quota,
          expiresInDays,
          { rate_limit_5h: 0, rate_limit_1d: 0, rate_limit_7d: 0 }
        )
        created.push(newKey.key)
      }
      createdKeys.value = created
      appStore.showSuccess(t('keys.keyCreatedSuccess'))
      if (onboardingStore.isCurrentStep('[data-tour="key-form-submit"]')) {
        onboardingStore.nextStep(500)
      }
    }
    closeModals()
    await loadApiKeys()
  } catch (error: any) {
    const errorMsg = error.response?.data?.detail || error.message || t('keys.failedToSave')
    appStore.showError(errorMsg)
  } finally {
    submitting.value = false
  }
}

/**
 * 处理删除 API Key 的操作
 * 优化：错误处理改进，优先显示后端返回的具体错误消息（如权限不足等），
 * 若后端未返回消息则显示默认的国际化文本
 */
const handleDelete = async () => {
  if (!selectedKey.value) return

  try {
    const deleteFn = isAdmin.value ? keysAPI.removeAdmin : keysAPI.delete
    await deleteFn(selectedKey.value.id)
    appStore.showSuccess(t('keys.keyDeletedSuccess'))
    showDeleteDialog.value = false
    loadApiKeys()
  } catch (error: any) {
    // 优先使用后端返回的错误消息，提供更具体的错误信息给用户
    const errorMsg = error?.message || t('keys.failedToDelete')
    appStore.showError(errorMsg)
  }
}

const closeModals = () => {
  showCreateModal.value = false
  showEditModal.value = false
  // Never leave the form's in-flight flag set: the create/edit dialog renders its submit
  // button disabled and labelled "Saving…" while `submitting` is true, so a close that
  // raced an in-flight submit would leave a dead-looking dialog behind.
  submitting.value = false
  selectedKey.value = null
  formData.value = {
    user_id: null,
    name: '',
    group_ids: [],
    enable_model_restriction: false,
    allowed_models: [],
    status: 'active',
    use_custom_key: false,
    custom_key: '',
    enable_ip_restriction: false,
    ip_whitelist: '',
    ip_blacklist: '',
    enable_quota: false,
    unlimited_quota: true,
    quantity: 1,
    quota: null,
    enable_rate_limit: false,
    rate_limit_5h: null,
    rate_limit_1d: null,
    rate_limit_7d: null,
    enable_expiration: false,
    expiration_preset: '30',
    expiration_date: ''
  }
}

// Show reset quota confirmation dialog
const confirmResetQuota = () => {
  showResetQuotaDialog.value = true
}

// Set expiration date based on quick select days
const setExpirationDays = (days: number) => {
  formData.value.expiration_preset = days.toString() as '7' | '30' | '90'
  const expDate = new Date()
  expDate.setDate(expDate.getDate() + days)
  formData.value.expiration_date = formatDateTimeLocal(expDate.toISOString())
}

// Reset quota used for an API key
const resetQuotaUsed = async () => {
  if (!selectedKey.value) return
  showResetQuotaDialog.value = false
  try {
    const updateFn = isAdmin.value ? keysAPI.updateAdmin : keysAPI.update
    await updateFn(selectedKey.value.id, { reset_quota: true })
    appStore.showSuccess(t('keys.quotaResetSuccess'))
    // Update local state
    if (selectedKey.value) {
      selectedKey.value.quota_used = 0
    }
  } catch (error: any) {
    const errorMsg = error.response?.data?.detail || t('keys.failedToResetQuota')
    appStore.showError(errorMsg)
  }
}

// Show reset rate limit confirmation dialog (from edit modal)
const confirmResetRateLimit = () => {
  showResetRateLimitDialog.value = true
}

// Show reset rate limit confirmation dialog (from table row)
const confirmResetRateLimitFromTable = (row: ApiKey) => {
  selectedKey.value = row
  showResetRateLimitDialog.value = true
}

// Reset rate limit usage for an API key
const resetRateLimitUsage = async () => {
  if (!selectedKey.value) return
  showResetRateLimitDialog.value = false
  try {
    const updateFn = isAdmin.value ? keysAPI.updateAdmin : keysAPI.update
    await updateFn(selectedKey.value.id, { reset_rate_limit_usage: true })
    appStore.showSuccess(t('keys.rateLimitResetSuccess'))
    // Refresh key data
    await loadApiKeys()
    // Update the editing key with fresh data
    const refreshedKey = apiKeys.value.find(k => k.id === selectedKey.value!.id)
    if (refreshedKey) {
      selectedKey.value = refreshedKey
    }
  } catch (error: any) {
    const errorMsg = error.response?.data?.detail || t('keys.failedToResetRateLimit')
    appStore.showError(errorMsg)
  }
}

const importToCcswitch = async (row: ApiKey) => {
  // Import needs the real key value — hash-only keys cannot be imported.
  const value = await revealKey(row)
  if (!value) return
  const fullRow: ApiKey = { ...row, key: value }
  const platform = fullRow.group?.platform || 'anthropic'

  // For antigravity platform, show client selection dialog
  if (platform === 'antigravity') {
    pendingCcsRow.value = fullRow
    showCcsClientSelect.value = true
    return
  }

  // For other platforms, execute directly
  executeCcsImport(fullRow, platform === 'gemini' ? 'gemini' : 'claude')
}

const executeCcsImport = (row: ApiKey, clientType: 'claude' | 'gemini') => {
  const baseUrl = getPublicApiBaseUrl(publicSettings.value?.api_base_url)
  const platform = row.group?.platform || 'anthropic'

  // Determine app name and endpoint based on platform and client type
  let app: string
  let endpoint: string

  if (platform === 'antigravity') {
    // Antigravity always uses /antigravity suffix
    app = clientType === 'gemini' ? 'gemini' : 'claude'
    endpoint = `${baseUrl}/antigravity`
  } else {
    switch (platform) {
      case 'openai':
        app = 'codex'
        endpoint = baseUrl
        break
      case 'gemini':
        app = 'gemini'
        endpoint = baseUrl
        break
      default: // anthropic
        app = 'claude'
        endpoint = baseUrl
    }
  }

  const usageScript = `({
    request: {
      url: "{{baseUrl}}/v1/usage",
      method: "GET",
      headers: { "Authorization": "Bearer {{apiKey}}" }
    },
    extractor: function(response) {
      const remaining = response?.remaining ?? response?.quota?.remaining ?? response?.balance;
      const unit = response?.unit ?? response?.quota?.unit ?? "USD";
      return {
        isValid: response?.is_active ?? response?.isValid ?? true,
        remaining,
        unit
      };
    }
  })`
  const providerName = (publicSettings.value?.site_name || 'sub2api').trim() || 'sub2api'

  const params = new URLSearchParams({
    resource: 'provider',
    app: app,
    name: providerName,
    homepage: baseUrl,
    endpoint: endpoint,
    apiKey: row.key,
    configFormat: 'json',
    usageEnabled: 'true',
    usageScript: btoa(usageScript),
    usageAutoInterval: '30'
  })
  const deeplink = `ccswitch://v1/import?${params.toString()}`

  try {
    window.open(deeplink, '_self')

    // Check if the protocol handler worked by detecting if we're still focused
    setTimeout(() => {
      if (document.hasFocus()) {
        // Still focused means the protocol handler likely failed
        appStore.showError(t('keys.ccSwitchNotInstalled'))
      }
    }, 100)
  } catch (error) {
    appStore.showError(t('keys.ccSwitchNotInstalled'))
  }
}

const handleCcsClientSelect = (clientType: 'claude' | 'gemini') => {
  if (pendingCcsRow.value) {
    executeCcsImport(pendingCcsRow.value, clientType)
  }
  showCcsClientSelect.value = false
  pendingCcsRow.value = null
}

const closeCcsClientSelect = () => {
  showCcsClientSelect.value = false
  pendingCcsRow.value = null
}

function formatResetTime(resetAt: string | null): string {
  if (!resetAt) return ''
  const diff = new Date(resetAt).getTime() - now.value.getTime()
  if (diff <= 0) return t('keys.resetNow')
  const days = Math.floor(diff / 86400000)
  const hours = Math.floor((diff % 86400000) / 3600000)
  const mins = Math.floor((diff % 3600000) / 60000)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m`
}

onMounted(() => {
  loadApiKeys()
  loadGroups()
  loadUserGroupRates()
  loadPublicSettings()
  if (isAdmin.value) {
    loadAllUsers()
  }
  document.addEventListener('click', closeGroupSelector)
  resetTimer = setInterval(() => { now.value = new Date() }, 60000)
})

onUnmounted(() => {
  document.removeEventListener('click', closeGroupSelector)
  if (resetTimer) clearInterval(resetTimer)
})
</script>
