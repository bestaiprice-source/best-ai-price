<template>
  <div>
    <!-- `select` variant: a single-line picker that opens a searchable model
         list (the OpenLux/provider-console layout). -->
    <div v-if="variant === 'select'" class="relative">
      <button
        type="button"
        class="flex w-full items-center justify-between gap-2 rounded-lg border border-gray-300 bg-white px-3 py-2 text-left text-sm dark:border-dark-500 dark:bg-dark-700"
        @click="toggleDropdown"
      >
        <span v-if="modelValue.length === 0" class="truncate text-gray-400">
          {{ t('keys.modelPickerPlaceholder') }}
        </span>
        <span v-else class="truncate text-gray-900 dark:text-white">
          {{ t('keys.modelPickerSelected', { count: modelValue.length }) }}
        </span>
        <Icon name="chevronDown" size="sm" class="h-4 w-4 shrink-0 text-gray-400" />
      </button>
      <div
        v-if="showDropdown"
        class="absolute left-0 right-0 top-full z-50 mt-1 overflow-hidden rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-700"
      >
        <div class="flex items-center gap-2 border-b border-gray-200 px-3 py-2 dark:border-dark-600">
          <Icon name="search" size="sm" class="h-4 w-4 shrink-0 text-gray-400" />
          <input
            ref="searchInput"
            v-model="searchQuery"
            type="text"
            class="w-full bg-transparent text-sm text-gray-900 outline-none placeholder:text-gray-400 dark:text-white"
            :placeholder="t('keys.modelPickerSearch')"
            @click.stop
          />
        </div>
        <div class="max-h-60 overflow-auto py-1">
          <button
            v-for="model in filteredModels"
            :key="model.value"
            type="button"
            class="flex w-full items-center gap-2.5 px-3 py-1.5 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-600"
            @click="toggleModel(model.value)"
          >
            <span
              :class="[
                'h-4 w-4 shrink-0 rounded-full border transition-colors',
                modelValue.includes(model.value)
                  ? 'border-[5px] border-primary-500'
                  : 'border-gray-300 dark:border-dark-500'
              ]"
            />
            <span class="truncate text-gray-900 dark:text-white">{{ model.value }}</span>
          </button>
          <div v-if="filteredModels.length === 0" class="px-3 py-4 text-center text-sm text-gray-500">
            {{ t('keys.modelPickerNoResults') }}
          </div>
        </div>
      </div>
    </div>

    <!-- Multi-select Dropdown (default `chips` variant) -->
    <div v-else class="relative mb-3">
      <div
        @click="toggleDropdown"
        class="cursor-pointer rounded-lg border border-gray-300 bg-white px-3 py-2 dark:border-dark-500 dark:bg-dark-700"
      >
        <div class="grid grid-cols-2 gap-1.5">
          <span
            v-for="model in modelValue"
            :key="model"
            class="inline-flex items-center justify-between gap-1 rounded bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-dark-600 dark:text-gray-300"
          >
            <span class="flex items-center gap-1 truncate">
              <ModelIcon :model="model" size="14px" />
              <span class="truncate">{{ model }}</span>
            </span>
            <button
              type="button"
              @click.stop="removeModel(model)"
              class="shrink-0 rounded-full hover:bg-gray-200 dark:hover:bg-dark-500"
            >
              <Icon name="x" size="xs" class="h-3.5 w-3.5" :stroke-width="2" />
            </button>
          </span>
        </div>
        <div class="mt-2 flex items-center justify-between border-t border-gray-200 pt-2 dark:border-dark-600">
          <span class="text-xs text-gray-400">{{ t('admin.accounts.modelCount', { count: modelValue.length }) }}</span>
          <svg class="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
          </svg>
        </div>
      </div>
      <!-- Dropdown List -->
      <div
        v-if="showDropdown"
        class="absolute left-0 right-0 top-full z-50 mt-1 rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-700"
      >
        <div class="sticky top-0 border-b border-gray-200 bg-white p-2 dark:border-dark-600 dark:bg-dark-700">
          <input
            v-model="searchQuery"
            type="text"
            class="input w-full text-sm"
            :placeholder="t('admin.accounts.searchModels')"
            @click.stop
          />
        </div>
        <div class="max-h-52 overflow-auto">
          <button
            v-for="model in filteredModels"
            :key="model.value"
            type="button"
            @click="toggleModel(model.value)"
            class="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-600"
          >
            <span
              :class="[
                'flex h-4 w-4 shrink-0 items-center justify-center rounded border',
                modelValue.includes(model.value)
                  ? 'border-primary-500 bg-primary-500 text-white'
                  : 'border-gray-300 dark:border-dark-500'
              ]"
            >
              <svg v-if="modelValue.includes(model.value)" class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7" />
              </svg>
            </span>
            <ModelIcon :model="model.value" size="18px" />
            <span class="truncate text-gray-900 dark:text-white">{{ model.value }}</span>
          </button>
          <div v-if="filteredModels.length === 0" class="px-3 py-4 text-center text-sm text-gray-500">
            {{ t('admin.accounts.noMatchingModels') }}
          </div>
        </div>
      </div>
    </div>

    <!-- Quick Actions (default variant) -->
    <div v-if="variant !== 'select'" class="mb-4 flex flex-wrap gap-2">
      <button
        type="button"
        @click="fillRelated"
        class="rounded-lg border border-blue-200 px-3 py-1.5 text-sm text-blue-600 hover:bg-blue-50 dark:border-blue-800 dark:text-blue-400 dark:hover:bg-blue-900/30"
      >
        {{ t('admin.accounts.fillRelatedModels') }}
      </button>
      <button
        type="button"
        @click="clearAll"
        class="rounded-lg border border-red-200 px-3 py-1.5 text-sm text-red-600 hover:bg-red-50 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-900/30"
      >
        {{ t('admin.accounts.clearAllModels') }}
      </button>
    </div>

    <!-- Family presets (select variant): fill / clear a whole model family. -->
    <div v-if="variant === 'select' && presets.length > 0" class="mt-3">
      <div class="mb-1.5 flex items-center justify-between">
        <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('keys.modelPresetsTitle') }}</span>
        <button
          v-if="modelValue.length > 0"
          type="button"
          class="text-xs text-gray-500 hover:text-red-500 dark:text-gray-400"
          @click="clearAll"
        >
          {{ t('keys.modelPickerClear') }}
        </button>
      </div>
      <div class="flex flex-wrap gap-2">
        <button
          v-for="preset in presets"
          :key="preset.key"
          type="button"
          class="rounded-full border px-3 py-1 text-xs font-medium transition-colors"
          :class="
            isPresetActive(preset)
              ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
              : 'border-gray-200 text-gray-600 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-600'
          "
          @click="applyPreset(preset)"
        >
          {{ preset.label }}
        </button>
      </div>
    </div>

    <!-- Custom Model Input -->
    <div class="mt-3">
      <label class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.accounts.customModelName') }}</label>
      <div class="flex gap-2">
        <input
          v-model="customModel"
          type="text"
          class="input flex-1"
          :placeholder="t('admin.accounts.enterCustomModelName')"
          @keydown.enter.prevent="handleEnter"
          @compositionstart="isComposing = true"
          @compositionend="isComposing = false"
        />
        <button
          type="button"
          @click="addCustom"
          class="rounded-lg bg-primary-50 px-4 py-2 text-sm font-medium text-primary-600 hover:bg-primary-100 dark:bg-primary-900/30 dark:text-primary-400 dark:hover:bg-primary-900/50"
        >
          {{ t('admin.accounts.addModel') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import ModelIcon from '@/components/common/ModelIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import { allModels, getModelsByPlatform } from '@/composables/useModelWhitelist'

const { t } = useI18n()

/** A named quick-fill preset rendered as a chip (`{ key, label, models }`). */
export interface ModelWhitelistPreset {
  key: string
  label: string
  models: string[]
}

const props = defineProps<{
  modelValue: string[]
  platform?: string
  platforms?: string[]
  /**
   * `chips` (default) keeps the account-modal layout; `select` renders the
   * single-line picker that opens a searchable model list, matching the
   * provider consoles (used at API-key creation).
   */
  variant?: 'chips' | 'select'
  /**
   * Explicit option pool. When provided (non-empty) it REPLACES the curated
   * per-platform list — the key form passes the live public catalog merged with
   * the curated family lists, so a key can only ever be restricted to models we
   * actually publish.
   */
  options?: string[]
  /** Quick-fill family presets (select variant only). */
  presets?: ModelWhitelistPreset[]
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string[]]
}>()

const appStore = useAppStore()

const showDropdown = ref(false)
const searchQuery = ref('')
const customModel = ref('')
const isComposing = ref(false)
const searchInput = ref<HTMLInputElement | null>(null)

const variant = computed(() => props.variant ?? 'chips')
const presets = computed<ModelWhitelistPreset[]>(() => props.presets ?? [])

const normalizedPlatforms = computed(() => {
  const rawPlatforms =
    props.platforms && props.platforms.length > 0
      ? props.platforms
      : props.platform
        ? [props.platform]
        : []

  return Array.from(
    new Set(
      rawPlatforms
        .map(platform => platform?.trim())
        .filter((platform): platform is string => Boolean(platform))
    )
  )
})

const availableOptions = computed(() => {
  if (props.options && props.options.length > 0) {
    return props.options.map(model => ({ value: model, label: model }))
  }
  if (normalizedPlatforms.value.length === 0) {
    return allModels
  }

  const allowedModels = new Set<string>()
  for (const platform of normalizedPlatforms.value) {
    for (const model of getModelsByPlatform(platform)) {
      allowedModels.add(model)
    }
  }

  return allModels.filter(model => allowedModels.has(model.value))
})

const filteredModels = computed(() => {
  const query = searchQuery.value.toLowerCase().trim()
  if (!query) return availableOptions.value
  return availableOptions.value.filter(
    m => m.value.toLowerCase().includes(query) || m.label.toLowerCase().includes(query)
  )
})

const toggleDropdown = () => {
  showDropdown.value = !showDropdown.value
  if (showDropdown.value) {
    if (variant.value === 'select') {
      nextTick(() => searchInput.value?.focus())
    }
    return
  }
  searchQuery.value = ''
}

const removeModel = (model: string) => {
  emit('update:modelValue', props.modelValue.filter(m => m !== model))
}

const toggleModel = (model: string) => {
  if (props.modelValue.includes(model)) {
    removeModel(model)
  } else {
    emit('update:modelValue', [...props.modelValue, model])
  }
}

const addCustom = () => {
  const model = customModel.value.trim()
  if (!model) return
  if (props.modelValue.includes(model)) {
    appStore.showInfo(t('admin.accounts.modelExists'))
    return
  }
  emit('update:modelValue', [...props.modelValue, model])
  customModel.value = ''
}

const handleEnter = () => {
  if (!isComposing.value) addCustom()
}

const fillRelated = () => {
  const newModels = [...props.modelValue]
  for (const platform of normalizedPlatforms.value) {
    for (const model of getModelsByPlatform(platform)) {
      if (!newModels.includes(model)) {
        newModels.push(model)
      }
    }
  }
  emit('update:modelValue', newModels)
}

const clearAll = () => {
  emit('update:modelValue', [])
}

/** Whether every model of a preset is already on the allowlist. */
const isPresetActive = (preset: ModelWhitelistPreset): boolean =>
  preset.models.length > 0 && preset.models.every(model => props.modelValue.includes(model))

/** One click fills the whole family; clicking an active preset removes it. */
const applyPreset = (preset: ModelWhitelistPreset) => {
  if (preset.models.length === 0) return
  if (isPresetActive(preset)) {
    const drop = new Set(preset.models)
    emit('update:modelValue', props.modelValue.filter(model => !drop.has(model)))
    return
  }
  emit('update:modelValue', Array.from(new Set([...props.modelValue, ...preset.models])))
}

</script>
