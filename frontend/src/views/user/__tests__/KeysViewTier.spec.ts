import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

const { createKey, getAvailableGroups } = vi.hoisted(() => ({
  createKey: vi.fn().mockResolvedValue({ key: 'sk-created' }),
  getAvailableGroups: vi.fn().mockResolvedValue([
    { id: 5, name: 'smart', platform: 'yunwu' },
    { id: 6, name: 'fast', platform: 'yunwu' }
  ])
}))

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/i18n', () => ({ i18n: { global: { t: (k: string) => k, locale: { value: 'en' } } } }))
vi.mock('@/api', () => ({
  keysAPI: {
    list: vi.fn().mockResolvedValue({ data: [], total: 0 }),
    create: createKey,
    update: vi.fn(),
    remove: vi.fn()
  },
  authAPI: {
    getUsers: vi.fn().mockResolvedValue({ data: [], total: 0 }),
    getPublicSettings: vi.fn().mockResolvedValue({})
  },
  usageAPI: { getByDateRange: vi.fn().mockResolvedValue({ data: [], total: 0 }) },
  userGroupsAPI: {
    getAvailable: getAvailableGroups,
    getUserGroupRates: vi.fn().mockResolvedValue({})
  }
}))
vi.mock('@/api/admin', () => ({ adminAPI: { users: { list: vi.fn().mockResolvedValue({ data: [], total: 0 }) } } }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn(), cachedPublicSettings: {} })
}))
vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({ isCurrentStep: () => false, nextStep: vi.fn() })
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isAdmin: false, user: { id: 1, balance: 10 } })
}))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))
vi.mock('@/composables/usePersistedPageSize', () => ({ getPersistedPageSize: () => 20 }))

import KeysView from '../KeysView.vue'

const slot = { template: '<div><slot name="actions" /><slot name="filters" /><slot /></div>' }
const passThrough = { template: '<div><slot /></div>' }

const mountView = async () => {
  const wrapper = mount(KeysView, {
    global: {
      stubs: {
        AppLayout: passThrough,
        TablePageLayout: slot,
        DataTable: { props: ['columns', 'data'], template: '<div class="dt"><slot /><slot name="empty" /></div>' },
        Pagination: true,
        ConfirmDialog: true,
        EmptyState: true,
        Select: true,
        SearchInput: true,
        Icon: { props: ['name'], template: '<i />' },
        UseKeyModal: true,
        GroupBadge: { props: ['name'], template: '<span>{{ name }}</span>' },
        GroupOptionItem: true,
        ModelWhitelistSelector: true,
        QuickStart: true,
        BaseDialog: { props: { show: Boolean }, template: '<div v-if="show" class="dlg"><slot /></div>' }
      }
    }
  })
  await new Promise((resolve) => setTimeout(resolve, 0))
  await nextTick()
  return wrapper
}

const byText = (wrapper: any, text: string) =>
  wrapper.findAll('button').find((b: any) => b.text() === text)

describe('KeysView billing tier picker', () => {
  beforeEach(() => {
    createKey.mockClear()
    getAvailableGroups.mockReset()
  })

  it('offers Smart and Fast when the admin created those groups, Smart selected first', async () => {
    getAvailableGroups.mockResolvedValue([
      { id: 5, name: 'smart', platform: 'yunwu' },
      { id: 6, name: 'fast', platform: 'yunwu' }
    ])
    const wrapper = await mountView()
    await byText(wrapper, 'keys.createKey').trigger('click')
    await nextTick()

    // The create dialog is open and shows the tier picker instead of a group picker.
    expect(wrapper.find('.dlg').exists()).toBe(true)
    expect(wrapper.text()).toContain('keys.tierLabel')
    expect(byText(wrapper, 'keys.tierSmart')).toBeTruthy()
    expect(byText(wrapper, 'keys.tierFast')).toBeTruthy()
    // Smart is the default selection (primary styling on the Smart button).
    expect(byText(wrapper, 'keys.tierSmart').classes().join(' ')).toContain('border-primary-500')
    wrapper.unmount()
  })

  it('offers both tiers even when the deployment created no tier group', async () => {
    // Only an ordinary group exists: the tier is a property of the key, so the
    // picker must still be there.
    getAvailableGroups.mockResolvedValue([{ id: 9, name: 'standard', platform: 'standard' }])
    const wrapper = await mountView()
    await byText(wrapper, 'keys.createKey').trigger('click')
    await nextTick()

    expect(wrapper.find('.dlg').exists()).toBe(true)
    expect(byText(wrapper, 'keys.tierSmart')).toBeTruthy()
    expect(byText(wrapper, 'keys.tierFast')).toBeTruthy()
    expect(byText(wrapper, 'keys.tierSmart').classes().join(' ')).toContain('border-primary-500')
    wrapper.unmount()
  })

  it('binds a created key to the picked tier group and sends the tier', async () => {
    getAvailableGroups.mockResolvedValue([
      { id: 5, name: 'smart', platform: 'yunwu' },
      { id: 6, name: 'fast', platform: 'yunwu' }
    ])
    const wrapper = await mountView()
    await byText(wrapper, 'keys.createKey').trigger('click')
    await nextTick()

    await byText(wrapper, 'keys.tierFast').trigger('click')
    await nextTick()

    await wrapper.find('input[data-tour="key-form-name"]').setValue('My fast key')
    await wrapper.find('#key-form').trigger('submit')
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(createKey).toHaveBeenCalledTimes(1)
    const [name, groupId, groupIds, , , , , , , , , tier] = createKey.mock.calls[0]
    expect(name).toBe('My fast key')
    expect(groupId).toBe(6)
    expect(groupIds).toEqual([6])
    expect(tier).toBe('fast')
    wrapper.unmount()
  })

  it('sends the picked tier with the default group when there is no tier group', async () => {
    getAvailableGroups.mockResolvedValue([{ id: 9, name: 'standard', platform: 'standard' }])
    const wrapper = await mountView()
    await byText(wrapper, 'keys.createKey').trigger('click')
    await nextTick()

    await byText(wrapper, 'keys.tierFast').trigger('click')
    await nextTick()

    await wrapper.find('input[data-tour="key-form-name"]').setValue('My fast key')
    await wrapper.find('#key-form').trigger('submit')
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(createKey).toHaveBeenCalledTimes(1)
    const [name, groupId, groupIds, , , , , , , , , tier] = createKey.mock.calls[0]
    expect(name).toBe('My fast key')
    expect(groupId).toBe(9)
    expect(groupIds).toEqual([9])
    expect(tier).toBe('fast')
    wrapper.unmount()
  })
})
