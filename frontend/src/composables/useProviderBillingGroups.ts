import { ref, type Ref } from 'vue'
import { getBillingGroups } from '@/api/admin/providers'

/**
 * The billing groups a provider's own price list exposes.
 *
 * A provider bills a key by the group the key was issued in, so the account form
 * offers exactly the groups that provider advertises (with the multiplier that
 * will be charged) instead of asking the admin to type a group name from memory.
 */
export interface ProviderGroupOption {
  /** The group name to store on the account. */
  value: string
  /** Display label, e.g. `Azure-Gpt-6 · 0.9x`. */
  label: string
  ratio: number
  models: number
}

export interface ProviderBillingGroups {
  options: Ref<ProviderGroupOption[]>
  loading: Ref<boolean>
  /** Fetch (or refresh) the provider's groups; failures leave the list empty. */
  load: (provider: string, baseUrl?: string) => Promise<void>
  /** Options for a field, keeping a pinned group the provider no longer lists. */
  optionsWith: (pinned: string) => ProviderGroupOption[]
}

export function useProviderBillingGroups(): ProviderBillingGroups {
  const options = ref<ProviderGroupOption[]>([])
  const loading = ref(false)
  let loadedKey = ''

  const load = async (provider: string, baseUrl?: string) => {
    const key = `${provider}|${baseUrl ?? ''}`
    if (!provider || key === loadedKey) return
    loading.value = true
    try {
      const res = await getBillingGroups({ provider, base_url: baseUrl || undefined })
      options.value = (res?.groups ?? []).map((group) => ({
        value: group.name,
        label: `${group.name} · ${group.ratio}x`,
        ratio: group.ratio,
        models: group.models
      }))
      loadedKey = key
    } catch {
      // A provider that cannot be reached simply offers no presets — the field
      // still accepts the default/auto-detected value.
      options.value = []
      loadedKey = ''
    } finally {
      loading.value = false
    }
  }

  const optionsWith = (pinned: string): ProviderGroupOption[] => {
    const list = [...options.value]
    const name = (pinned ?? '').trim()
    if (name && !list.some((option) => option.value === name)) {
      // Keep a group that is already stored (or no longer advertised) selectable,
      // so opening the form can never silently reset it to "not set".
      list.unshift({ value: name, label: name, ratio: Number.NaN, models: 0 })
    }
    return list
  }

  return { options, loading, load, optionsWith }
}
