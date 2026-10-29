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
  it('always offers both tiers and keeps the default group when the admin created no tier group', () => {
    const t = setup([group(1, 'yunwu'), group(2, 'default')])
    expect(t.tierOptions).toEqual(['smart', 'fast'])
    expect(t.hasTierGroups.value).toBe(false)
    expect(t.tierGroups.value).toEqual([])
    expect(t.userTier.value).toBe('smart')
    // No tier group -> the key is bound to the ordinary default group and the
    // tier travels on the key itself.
    expect(t.selectedGroupId.value).toBe(99)
  })

  it('keeps offering both tiers when only one tier group exists', () => {
    const t = setup([group(6, 'fast')])
    expect(t.tierOptions).toEqual(['smart', 'fast'])
    expect(t.tierGroups.value).toEqual(['fast'])
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

  it('falls back to the default group for a tier the deployment has no group for', async () => {
    // Only `fast` exists: keeping the smart selection must not bind the key to
    // the fast group.
    const t = setup([group(6, 'fast')])
    expect(t.userTier.value).toBe('smart')
    expect(t.selectedGroupId.value).toBe(99)

    t.userTier.value = 'fast'
    await nextTick()
    expect(t.selectedGroupId.value).toBe(6)
  })

  it('resets the selection to smart on syncTierDefault', async () => {
    const t = setup([group(5, 'smart'), group(6, 'fast')])
    t.userTier.value = 'fast'
    await nextTick()
    t.syncTierDefault()
    await nextTick()
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

describe('useKeyTier effectiveTierForKey', () => {
  it('prefers the tier stored on the key', () => {
    const t = setup([])
    expect(t.effectiveTierForKey({ tier: 'fast', group: { name: 'smart' } })).toBe('fast')
    expect(t.effectiveTierForKey({ tier: 'SMART' })).toBe('smart')
  })

  it('falls back to the key group names for a key stored before the tier existed', () => {
    const t = setup([])
    expect(t.effectiveTierForKey({ tier: '', group: { name: 'Fast' } })).toBe('fast')
    expect(t.effectiveTierForKey({ tier: '', groups: [{ name: 'yunwu' }, { name: 'fast' }] })).toBe('fast')
  })

  it('is smart when nothing identifies a tier', () => {
    const t = setup([])
    expect(t.effectiveTierForKey({ tier: '', group: { name: 'yunwu' } })).toBe('smart')
    expect(t.effectiveTierForKey({})).toBe('smart')
    expect(t.effectiveTierForKey(null)).toBe('smart')
  })
})
