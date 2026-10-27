import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn().mockResolvedValue(true) })
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ cachedPublicSettings: { api_base_url: 'https://api.example.com' } })
}))

vi.mock('@/components/icons/Icon.vue', () => ({
  default: { name: 'Icon', props: ['name'], template: '<i />' }
}))

vi.mock('@/api', () => ({
  keysAPI: { reveal: vi.fn().mockResolvedValue('sk-revealed') }
}))

import UserDashboardOnboarding from '../UserDashboardOnboarding.vue'

const mountOnboarding = () =>
  mount(UserDashboardOnboarding, {
    props: { hasKey: true, hasBalance: true, hasRequest: false, firstKey: null },
    global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
  })

const languageTab = (wrapper: ReturnType<typeof mountOnboarding>, label: string) =>
  wrapper.findAll('button').find((button) => button.text().trim() === label)

describe('UserDashboardOnboarding', () => {
  it('offers the first request in Python, Node.js and cURL', async () => {
    const wrapper = mountOnboarding()

    for (const label of ['Python', 'Node.js', 'cURL']) {
      expect(languageTab(wrapper, label)).toBeTruthy()
    }
  })

  it('swaps the snippet when another language tab is picked', async () => {
    const wrapper = mountOnboarding()
    const snippet = () => wrapper.find('pre code').text()

    // cURL is the default.
    expect(snippet()).toContain('curl https://api.example.com/v1/chat/completions')
    expect(snippet()).toContain('Authorization: Bearer <YOUR_API_KEY>')

    await languageTab(wrapper, 'Python')!.trigger('click')
    expect(snippet()).toContain('from openai import OpenAI')
    expect(snippet()).toContain('api_key="<YOUR_API_KEY>"')

    await languageTab(wrapper, 'Node.js')!.trigger('click')
    expect(snippet()).toContain("import OpenAI from 'openai'")
    expect(snippet()).toContain("apiKey: '<YOUR_API_KEY>'")
  })

  it('keeps the placeholder in the snippet — the dashboard never prints a secret on its own', () => {
    const wrapper = mountOnboarding()
    expect(wrapper.find('pre code').text()).toContain('<YOUR_API_KEY>')
  })
})
