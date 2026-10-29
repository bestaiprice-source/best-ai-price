import { computed, ref, type ComputedRef, type Ref } from 'vue'

/**
 * Billing tiers for unofficial (yunwu) providers.
 *
 * A provider issues its API keys inside two billing groups — `fast` and `smart`
 * — which differ in speed and price. The tier IS the group name, so an API key
 * bound to the `fast`/`smart` group makes the gateway use the matching provider
 * key and price. This composable owns the regular-user side of that choice:
 * which tier is selected, which groups can be picked, and which group id the key
 * is finally bound to.
 */
export const TIER_GROUP_NAMES = ['smart', 'fast'] as const
export type UserTier = (typeof TIER_GROUP_NAMES)[number]

export interface TierGroupLike {
  id: number
  name: string
}

export interface KeyTierSelection {
  /** Currently selected tier (default smart). */
  userTier: Ref<UserTier>
  /** Tier groups the user may pick, in preference order. */
  tierGroups: ComputedRef<UserTier[]>
  /** Whether any tier group exists at all. */
  hasTierGroups: ComputedRef<boolean>
  /** Group id the key must be bound to. */
  selectedGroupId: ComputedRef<number | null>
  /** Point the selection at a tier the deployment actually offers. */
  syncTierDefault: () => void
}

/**
 * @param groups         groups available to the current user
 * @param isAdmin        an admin picks arbitrary groups, so the tier is bypassed
 * @param defaultGroupId fallback group when no tier groups exist (the `yunwu` group)
 */
export function useKeyTier(
  groups: Ref<TierGroupLike[]>,
  isAdmin: Ref<boolean> | ComputedRef<boolean>,
  defaultGroupId: ComputedRef<number | null>
): KeyTierSelection {
  const userTier = ref<UserTier>('smart')

  const tierGroups = computed(() =>
    TIER_GROUP_NAMES.filter((name) => groups.value.some((group) => group.name.toLowerCase() === name))
  )

  const hasTierGroups = computed(() => tierGroups.value.length > 0)

  const selectedGroupId = computed<number | null>(() => {
    if (!isAdmin.value && hasTierGroups.value) {
      const match = groups.value.find((group) => group.name.toLowerCase() === userTier.value)
      if (match) return match.id
    }
    return defaultGroupId.value
  })

  const syncTierDefault = () => {
    if (isAdmin.value || !hasTierGroups.value) return
    if (!tierGroups.value.includes(userTier.value)) {
      userTier.value = tierGroups.value[0]
    }
  }

  return { userTier, tierGroups, hasTierGroups, selectedGroupId, syncTierDefault }
}
