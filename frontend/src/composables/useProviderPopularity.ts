import { ref, computed } from 'vue'
import providerPopularityAPI from '@/api/providerPopularity'

const popularityMap = ref<Record<string, number>>({})
const lastClickTimestamps = new Map<string, number>()
const isFetching = ref(false)

export function useProviderPopularity() {
  /**
   * Fetch all provider popularities from server
   */
  async function fetchPopularity() {
    if (isFetching.value) return
    isFetching.value = true
    try {
      const res = await providerPopularityAPI.getPopularity()
      if (res.data?.providers) {
        const nextMap: Record<string, number> = {}
        for (const item of res.data.providers) {
          nextMap[item.provider] = item.click_count
        }
        popularityMap.value = nextMap
      }
    } catch {
      // Graceful fallback if endpoint is unavailable
    } finally {
      isFetching.value = false
    }
  }

  /**
   * Get click count for a provider (defaults to 0)
   */
  function getClickCount(provider: string): number {
    return popularityMap.value[provider.toLowerCase()] || 0
  }

  /**
   * Record a click with optimistic local update and backend anti-abuse sync
   */
  async function trackClick(provider: string): Promise<boolean> {
    const slug = provider.toLowerCase().trim()
    if (!slug) return false

    const now = Date.now()
    const lastClicked = lastClickTimestamps.get(slug) || 0

    // Local debounce / anti-spam (prevent spamming network within 3s)
    if (now - lastClicked < 3000) {
      return false
    }
    lastClickTimestamps.set(slug, now)

    // Optimistic UI update
    popularityMap.value = {
      ...popularityMap.value,
      [slug]: (popularityMap.value[slug] || 0) + 1
    }

    try {
      const res = await providerPopularityAPI.recordClick(slug)
      if (res.data) {
        // Reconcile with verified count from server
        popularityMap.value = {
          ...popularityMap.value,
          [slug]: res.data.click_count
        }
        return res.data.counted
      }
    } catch {
      // Leave optimistic count or revert on network failure
    }
    return true
  }

  /**
   * Helper to sort an array of provider items by popularity
   */
  function sortByPopularity<T>(
    items: T[],
    getProvider: (item: T) => string,
    ascending = false
  ): T[] {
    return [...items].sort((a, b) => {
      const countA = getClickCount(getProvider(a))
      const countB = getClickCount(getProvider(b))
      return ascending ? countA - countB : countB - countA
    })
  }

  return {
    popularityMap: computed(() => popularityMap.value),
    isFetching,
    fetchPopularity,
    getClickCount,
    trackClick,
    sortByPopularity
  }
}

export default useProviderPopularity
