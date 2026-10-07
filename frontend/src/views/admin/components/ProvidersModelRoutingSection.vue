<template>
  <div class="space-y-6">
    <!-- Intro / info -->
    <div class="rounded-2xl border border-indigo-200/70 bg-indigo-50/60 p-5 text-sm text-indigo-900 dark:border-indigo-800/60 dark:bg-indigo-950/30 dark:text-indigo-200">
      <p class="font-medium">Primary / Backup routing is assigned automatically by price.</p>
      <p class="mt-1 text-indigo-800/80 dark:text-indigo-300/80">
        When no rule is set for a model, the system routes to the <b>cheapest</b> available provider
        as <b>Primary</b> and failsover to the <b>next cheapest</b> provider as <b>Backup</b>.
        Add a rule below only to pin specific providers (first = Primary, second = Backup, …).
      </p>
    </div>

    <!-- Group selector -->
    <div class="flex flex-wrap items-end gap-3">
      <div class="min-w-[240px]">
        <label class="mb-1 block text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">Group</label>
        <select v-model.number="selectedGroupId" class="input w-full" @change="loadSelectedGroup">
          <option :value="0">Select a Yunwu group…</option>
          <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }} ({{ g.platform }})</option>
        </select>
      </div>
      <div class="flex items-center gap-3">
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-xl border px-4 py-2 text-sm font-medium transition-colors"
          :class="enabled ? 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-300' : 'border-gray-200 bg-white text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300'"
          @click="enabled = !enabled"
        >
          Explicit routing: {{ enabled ? 'ON' : 'OFF' }}
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-xl bg-primary-600 px-4 py-2 text-sm font-medium text-white shadow-sm transition-all hover:bg-primary-500 disabled:opacity-50"
          :disabled="!selectedGroupId || saving"
          @click="save"
        >
          {{ saving ? 'Saving…' : 'Save routing' }}
        </button>
      </div>
    </div>

    <!-- Rules -->
    <div v-if="selectedGroupId" class="space-y-4">
      <p v-if="!enabled" class="text-xs text-gray-500 dark:text-gray-400">
        Explicit routing is OFF — the pool (cheapest-first) handles all models. Turn it ON to apply the rules below.
      </p>

      <div
        v-for="(rule, index) in rules"
        :key="rule._key"
        class="rounded-2xl border border-gray-200/80 bg-white/70 p-4 dark:border-dark-700/80 dark:bg-dark-800/70"
      >
        <div class="flex flex-wrap items-center gap-3">
          <div class="grow">
            <label class="mb-1 block text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">Model pattern</label>
            <input
              v-model="rule.pattern"
              type="text"
              class="input w-full"
              placeholder="e.g. claude-3-7-sonnet or claude-opus-*"
            />
          </div>
          <button
            type="button"
            class="mt-5 rounded-lg p-2 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-950/40"
            title="Remove rule"
            @click="removeRule(index)"
          >
            <Icon name="x" size="sm" />
          </button>
        </div>

        <div class="mt-3">
          <label class="mb-1 block text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            Providers (click to add — order = priority; #1 is Primary)
          </label>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="p in providerOptions"
              :key="p.id"
              type="button"
              class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-colors"
              :class="accountIndex(rule, p.id) >= 0
                ? 'border-primary-400 bg-primary-50 text-primary-700 dark:border-primary-700 dark:bg-primary-950/40 dark:text-primary-300'
                : 'border-gray-200 bg-white text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300'"
              @click="toggleAccount(rule, p.id)"
            >
              <span v-if="accountIndex(rule, p.id) >= 0" class="font-bold">#{{ accountIndex(rule, p.id) + 1 }}</span>
              {{ p.provider_name || ('#' + p.id) }}
            </button>
            <span v-if="providerOptions.length === 0" class="text-xs text-gray-400">No active providers loaded.</span>
          </div>
        </div>
      </div>

      <button
        type="button"
        class="inline-flex items-center gap-2 rounded-xl border border-dashed border-gray-300 px-4 py-2 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-800"
        @click="addRule"
      >
        <Icon name="plus" size="sm" />
        Add rule
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { getAll as listGroups, getById as getGroup, update as updateGroup } from '@/api/admin/groups'
import type { AdminGroup } from '@/types'
import type { Provider } from '@/api/admin/providers'

interface RoutingRule {
  _key: string
  pattern: string
  accountIds: number[]
}

const props = defineProps<{ providers: Provider[] }>()
const appStore = useAppStore()

const groups = ref<AdminGroup[]>([])
const selectedGroupId = ref<number>(0)
const enabled = ref(false)
const rules = ref<RoutingRule[]>([])
const saving = ref(false)

let ruleSeq = 0
const newRule = (): RoutingRule => ({ _key: `rule-${++ruleSeq}`, pattern: '', accountIds: [] })

const providerOptions = computed(() => props.providers.filter(p => p.status === 'active'))

const accountIndex = (rule: RoutingRule, id: number) => rule.accountIds.indexOf(id)

const toggleAccount = (rule: RoutingRule, id: number) => {
  const idx = rule.accountIds.indexOf(id)
  if (idx >= 0) {
    rule.accountIds.splice(idx, 1)
  } else {
    rule.accountIds.push(id)
  }
}

const addRule = () => rules.value.push(newRule())
const removeRule = (index: number) => rules.value.splice(index, 1)

const loadGroups = async () => {
  try {
    const list = await listGroups('yunwu')
    groups.value = Array.isArray(list) ? list : []
    if (groups.value.length > 0 && selectedGroupId.value === 0) {
      selectedGroupId.value = groups.value[0].id
      await loadSelectedGroup()
    }
  } catch (e) {
    console.warn('Failed to load groups for routing:', e)
  }
}

const loadSelectedGroup = async () => {
  if (!selectedGroupId.value) {
    enabled.value = false
    rules.value = []
    return
  }
  try {
    const group = await getGroup(selectedGroupId.value)
    enabled.value = !!group.model_routing_enabled
    const mapping = group.model_routing || {}
    rules.value = Object.entries(mapping).map(([pattern, ids]) => ({
      _key: `rule-${++ruleSeq}`,
      pattern,
      accountIds: Array.isArray(ids) ? [...ids] : []
    }))
    if (rules.value.length === 0) rules.value.push(newRule())
  } catch (e) {
    console.warn('Failed to load group routing:', e)
    appStore.showError('Failed to load group routing')
  }
}

const save = async () => {
  if (!selectedGroupId.value) return
  saving.value = true
  try {
    const mapping: Record<string, number[]> = {}
    let hasValid = false
    for (const rule of rules.value) {
      const pattern = rule.pattern.trim()
      const ids = rule.accountIds.filter(id => id > 0)
      if (!pattern || ids.length === 0) continue
      mapping[pattern] = ids
      hasValid = true
    }
    await updateGroup(selectedGroupId.value, {
      model_routing: hasValid ? mapping : null,
      model_routing_enabled: enabled.value
    })
    appStore.showSuccess('Model routing saved')
  } catch (e) {
    console.warn('Failed to save model routing:', e)
    appStore.showError('Failed to save model routing')
  } finally {
    saving.value = false
  }
}

onMounted(loadGroups)
</script>
