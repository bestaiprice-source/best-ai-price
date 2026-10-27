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

import QuickStart from '../QuickStart.vue'
import { buildApiSnippet } from '@/utils/apiSnippets'

const KEY = 'sk-test-key-1234567890'

const mountQuickStart = () =>
  mount(QuickStart, { props: { apiKey: KEY, moreKeys: 0 } })

describe('QuickStart', () => {
  it('defaults to OpenAI/Python and inserts the key and the public base URL', () => {
    const wrapper = mountQuickStart()

    const code = wrapper.find('pre code').text()
    expect(code).toContain(`api_key="${KEY}"`)
    expect(code).toContain('base_url="https://api.example.com/v1"')
    expect(code).toContain('https://api.example.com/v1/chat/completions')
  })

  it('switches the endpoint and snippet per protocol', async () => {
    const wrapper = mountQuickStart()
    const radios = wrapper.findAll('input[type="radio"]')

    // Anthropic: native /v1/messages, base URL without the /v1 suffix.
    await radios[1].setValue()
    let code = wrapper.find('pre code').text()
    expect(code).toContain('https://api.example.com/v1/messages')
    expect(code).toContain(`api_key="${KEY}"`)
    expect(code).toContain('base_url="https://api.example.com"')

    // Gemini: native v1beta generateContent.
    await radios[2].setValue()
    code = wrapper.find('pre code').text()
    expect(code).toContain(':generateContent')
    expect(code).toContain(`api_key="${KEY}"`)
    expect(code).toContain('https://api.example.com/v1beta/models/gemini-2.5-flash')
  })

  it('offers Python / Node.js / cURL snippets for the same key', async () => {
    const wrapper = mountQuickStart()
    const tabs = wrapper.findAll('button').filter((b) => ['Python', 'Node.js', 'cURL'].includes(b.text().trim()))
    expect(tabs.length).toBe(3)

    await tabs[1].trigger('click')
    expect(wrapper.find('pre code').text()).toContain(`apiKey: '${KEY}'`)

    await tabs[2].trigger('click')
    expect(wrapper.find('pre code').text()).toContain(`Authorization: Bearer ${KEY}`)
  })

  it('reports the number of additional keys minted in the same batch', () => {
    const wrapper = mount(QuickStart, { props: { apiKey: KEY, moreKeys: 4 } })
    expect(wrapper.text()).toContain('quickStart.moreKeys')
  })

  // Drift guard: the dialog and the dashboard onboarding must not grow their own
  // copies of the example — both render what the shared builder returns.
  it('renders exactly what the shared snippet builder produces', () => {
    const wrapper = mountQuickStart()

    expect(wrapper.find('pre code').text()).toBe(
      buildApiSnippet({
        protocol: 'openai',
        language: 'python',
        baseURL: 'https://api.example.com/v1',
        apiKey: KEY
      })
    )
  })
})