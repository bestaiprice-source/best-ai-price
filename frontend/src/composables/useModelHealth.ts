import { computed, ref } from 'vue'
import { getModelHealth, type ModelHealthItem } from '@/api/modelHealth'
import { modelHealthStatusMeta, type ModelHealthStatusMeta } from '@/constants/modelHealth'

/**
 * Shared model-health state.
 *
 * The dashboard health block and the model catalog cards show the same data, so
 * the fetch lives at module level and is shared: opening both screens issues ONE
 * request, and the two views can never disagree about a model's status.
 */

const items = ref<ModelHealthItem[]>([])
const windowDays = ref(7)
const loading = ref(false)
const loaded = ref(false)
const error = ref<string | null>(null)

let inflight: Promise<void> | null = null

/** Normalization keys to try for a catalog model id. */
export function modelLookupKeys(modelId: string | null | undefined): string[] {
  const raw = (modelId ?? '').trim().toLowerCase()
  if (!raw) return []
  const keys = [raw]
  // The gateway's resolver always also emits the vendor-less slug, so trying it
  // joins `anthropic/claude-3-7-sonnet` onto the monitor's upstream id.
  const slug = raw.split('/').pop()
  if (slug && slug !== raw) keys.push(slug)
  return keys
}

const byCandidate = computed(() => {
  const map = new Map<string, ModelHealthItem>()
  for (const item of items.value) {
    const keys = item.candidates?.length ? item.candidates : modelLookupKeys(item.model)
    for (const key of keys) {
      const current = map.get(key)
      // Prefer the healthiest match: several monitors may cover one model, and
      // the gateway routes to a healthy provider, so one bad provider must not
      // mark the whole model red.
      if (!current || statusRank(item) < statusRank(current)) map.set(key, item)
    }
  }
  return map
})

function statusRank(item: ModelHealthItem): number {
  return modelHealthStatusMeta(item.status).rank
}

/** Health of a catalog model, or null when no monitor covers it. */
function resolve(modelId: string | null | undefined): ModelHealthItem | null {
  for (const key of modelLookupKeys(modelId)) {
    const hit = byCandidate.value.get(key)
    if (hit) return hit
  }
  return null
}

function statusMetaOf(item: ModelHealthItem | null): ModelHealthStatusMeta {
  return modelHealthStatusMeta(item?.status)
}

/**
 * Fetches the health map once. Concurrent callers share one in-flight request;
 * `force` re-fetches (used by the explicit refresh affordance).
 */
async function load(force = false): Promise<void> {
  if (inflight) return inflight
  if (loaded.value && !force) return
  loading.value = true
  inflight = (async () => {
    try {
      const res = await getModelHealth(windowDays.value)
      items.value = res.items
      windowDays.value = res.window_days
      error.value = null
      loaded.value = true
    } catch (err) {
      // A missing/unavailable health endpoint must not break the dashboard or
      // the catalog: leave the block empty rather than throwing.
      error.value = err instanceof Error ? err.message : String(err)
    } finally {
      loading.value = false
      inflight = null
    }
  })()
  return inflight
}

/** Test hook: module-level state would otherwise leak between specs. */
export function resetModelHealthCache(): void {
  items.value = []
  loaded.value = false
  loading.value = false
  error.value = null
  inflight = null
}

export function useModelHealth() {
  return {
    items,
    windowDays,
    loading,
    loaded,
    error,
    load,
    resolve,
    statusMetaOf
  }
}
