import { computed, ref, type ComputedRef, type Ref } from 'vue'

/**
 * Billing tiers for unofficial (yunwu) providers.
 *
 * A provider issues its API keys inside two billing groups — `fast` and `smart`
 * — which differ in speed and price. The tier is an attribute of the API key
 * itself (`api_keys.tier`), so the choice is always offered to a regular user
 * and no longer depends on the deployment having created groups with those
 * names: the gateway keeps only nodes holding a key for that tier and the
 * runtime uses that tier's provider key.
 *
 * A key created before the tier column existed carries no tier, and the backend
 * then derives it from the key's group names — which is what
 * `effectiveTierForKey` mirrors for display.
 */
export const TIER_GROUP_NAMES = ['smart', 'fast'] as const
export type UserTier = (typeof TIER_GROUP_NAMES)[number]

export interface TierGroupLike {
  id: number
  name: string
}

/** The shape needed to display an existing key's effective tier. */
export interface KeyTierLike {
  tier?: string | null
  group?: { name?: string } | null
  groups?: { name?: string }[] | null
}

export interface KeyTierSelection {
  /** Currently selected tier (default smart). */
  userTier: Ref<UserTier>
  /** Every tier the picker offers — always both of them. */
  tierOptions: readonly UserTier[]
  /** Tier groups the deployment actually created (drives the key's group binding). */
  tierGroups: ComputedRef<UserTier[]>
  /** Whether any tier group exists at all. */
  hasTierGroups: ComputedRef<boolean>
  /** Group id the key must be bound to. */
  selectedGroupId: ComputedRef<number | null>
  /** Point the selection back at the default tier. */
  syncTierDefault: () => void
  /** The tier a stored key effectively runs on (explicit tier, else group names). */
  effectiveTierForKey: (key: KeyTierLike | null | undefined) => UserTier
}

const tierOfName = (name: string | undefined | null): UserTier | null => {
  const normalized = (name ?? '').trim().toLowerCase()
  return TIER_GROUP_NAMES.find((tier) => tier === normalized) ?? null
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

  // Always both tiers: the choice belongs to the key, not to the group list.
  const tierOptions = TIER_GROUP_NAMES

  const tierGroups = computed(() =>
    TIER_GROUP_NAMES.filter((name) => groups.value.some((group) => group.name.toLowerCase() === name))
  )

  const hasTierGroups = computed(() => tierGroups.value.length > 0)

  // When the deployment created a `smart`/`fast` group the key keeps being bound
  // to it (those groups may carry their own rate multiplier, and keys created
  // that way today rely on it); otherwise it binds to the default group and the
  // tier travels on the key itself.
  const selectedGroupId = computed<number | null>(() => {
    if (!isAdmin.value && tierGroups.value.includes(userTier.value)) {
      const match = groups.value.find((group) => group.name.toLowerCase() === userTier.value)
      if (match) return match.id
    }
    return defaultGroupId.value
  })

  const syncTierDefault = () => {
    if (isAdmin.value) return
    userTier.value = 'smart'
  }

  const effectiveTierForKey = (key: KeyTierLike | null | undefined): UserTier => {
    const stored = tierOfName(key?.tier)
    if (stored) return stored
    // Mirror of yunwu.TierFromGroupNames: the key's primary group first, then
    // the rest, and the first tier name found wins.
    const names: (string | undefined)[] = []
    if (key?.group) names.push(key.group.name)
    for (const group of key?.groups ?? []) names.push(group?.name)
    for (const name of names) {
      const tier = tierOfName(name)
      if (tier) return tier
    }
    return 'smart'
  }

  return { userTier, tierOptions, tierGroups, hasTierGroups, selectedGroupId, syncTierDefault, effectiveTierForKey }
}
