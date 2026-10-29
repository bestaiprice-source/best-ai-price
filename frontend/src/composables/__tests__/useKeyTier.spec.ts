import { describe, expect, it } from 'vitest'
import { computed, nextTick, ref } from 'vue'
import { useKeyTier, type TierGroupLike } from '../useKeyTier'

const group = (id: number, name: string): TierGroupLike => ({ id, name })

function setup(groups: TierGroupLike[], isAdmin = false, defaultId: number | null = 99) {
  const groupsRef = ref(groups)
  const adminRef = computed(() => isAdmin)
  const defaultRef = computed(() => defaultId)
  const tier = useKeyTier(groupsRef, adminRef, defaultRef)
  return { groupsRef, ...tier }
}

describe('useKeyTier', () => {
  it('offers no tier and keeps the default group when the admin created none', () => {
    const t = setup([group(1, 'yunwu'), group(2, 'default')])
    expect(t.hasTierGroups.value).toBe(false)
    expect(t.tierGroups.value).toEqual([])
    expect(t.userTier.value).toBe('smart')
    expect(t.selectedGroupId.value).toBe(99)
  })

  it('binds the key to the smart group by default and to fast once picked', async () => {
    const t = setup([group(5, 'smart'), group(6, 'fast')])
    expect(t.hasTierGroups.value).toBe(true)
    expect(t.tierGroups.value).toEqual(['smart', 'fast'])
    expect(t.userTier.value).toBe('smart')
    expect(t.selectedGroupId.value).toBe(5)

    t.userTier.value = 'fast'
    await nextTick()
    expect(t.selectedGroupId.value).toBe(6)
  })

  it('matches tier group names case-insensitively', () => {
    const t = setup([group(7, 'Fast'), group(8, 'SMART')])
    expect(t.tierGroups.value).toEqual(['smart', 'fast'])
    expect(t.selectedGroupId.value).toBe(8)
  })

  it('falls back to an offered tier when the default one is missing', async () => {
    // Only `fast` exists: the initial (smart) selection must move onto it.
    const t = setup([group(6, 'fast')])
    expect(t.selectedGroupId.value).toBe(99) // still the default group before syncing
    t.syncTierDefault()
    await nextTick()
    expect(t.userTier.value).toBe('fast')
    expect(t.selectedGroupId.value).toBe(6)
  })

  it('keeps the smart selection when it is available', () => {
    const t = setup([group(5, 'smart'), group(6, 'fast')])
    t.syncTierDefault()
    expect(t.userTier.value).toBe('smart')
    expect(t.selectedGroupId.value).toBe(5)
  })

  it('ignores the tier for admins, who pick arbitrary groups', async () => {
    const t = setup([group(5, 'smart'), group(6, 'fast')], true)
    expect(t.selectedGroupId.value).toBe(99)
    t.userTier.value = 'fast'
    await nextTick()
    expect(t.selectedGroupId.value).toBe(99)
    t.syncTierDefault()
    expect(t.userTier.value).toBe('fast') // untouched for admins
  })

  it('recomputes when the group list arrives after mount', async () => {
    const t = setup([])
    expect(t.hasTierGroups.value).toBe(false)
    expect(t.selectedGroupId.value).toBe(99)

    t.groupsRef.value = [group(5, 'smart'), group(6, 'fast')]
    await nextTick()
    expect(t.hasTierGroups.value).toBe(true)
    expect(t.selectedGroupId.value).toBe(5)
  })
})
