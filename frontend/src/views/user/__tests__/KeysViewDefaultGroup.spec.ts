import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

// The gateway publishes the pooled-provider group under its neutral "standard"
// label — a signed-in user never sees the internal provider-pool name. A key
// created by a regular user (with no billing-tier groups configured) must bind
// to that group.
const { createKey, getAvailableGroups } = vi.hoisted(() => ({
  createKey: vi.fn().mockResolvedValue({ key: 'sk-created' }),
  getAvailableGroups: vi.fn().mockResolvedValue([
    { id: 11, name: 'standard', platform: 'standard' },
    { id: 12, name: 'Anthropic Pro', platform: 'anthropic' }
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

describe('KeysView default group for regular users', () => {
  it('binds the key to the group published as "standard" when no tier groups exist', async () => {
    const wrapper = await mountView()
    await byText(wrapper, 'keys.createKey').trigger('click')
    await nextTick()

    // The tier is a property of the key, so the picker is offered even without
    // tier groups; the key still binds to the published "standard" group.
    expect(wrapper.text()).toContain('keys.tierLabel')
    expect(byText(wrapper, 'keys.tierSmart')).toBeTruthy()
    expect(byText(wrapper, 'keys.tierFast')).toBeTruthy()

    await wrapper.find('input[data-tour="key-form-name"]').setValue('My key')
    await wrapper.find('#key-form').trigger('submit')
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(createKey).toHaveBeenCalledTimes(1)
    const [, , groupIds, , , , , , , , , tier] = createKey.mock.calls[0]
    expect(groupIds).toEqual([11])
    expect(tier).toBe('smart')
    wrapper.unmount()
  })
})
