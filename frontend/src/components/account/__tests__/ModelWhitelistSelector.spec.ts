import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

// A message-format-safe stub: the component only needs the key back.
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
// `useModelWhitelist` reaches the API client, which builds the i18n instance.
vi.mock('@/i18n', () => ({ i18n: { global: { t: (k: string) => k, locale: { value: 'en' } } } }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showInfo: vi.fn(), showError: vi.fn() })
}))

import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'

const stubs = {
  ModelIcon: { props: ['model'], template: '<i />' },
  Icon: { props: ['name'], template: '<i />' }
}

const mountSelector = (props: Record<string, unknown>) =>
  mount(ModelWhitelistSelector, {
    props: { modelValue: [], ...props },
    global: { stubs }
  })

const lastEmitted = (wrapper: ReturnType<typeof mountSelector>): string[] =>
  wrapper.emitted('update:modelValue')!.at(-1)![0] as string[]

const optionButton = (wrapper: ReturnType<typeof mountSelector>, text: string) =>
  wrapper.findAll('button').find((b) => b.text() === text)

describe('ModelWhitelistSelector', () => {
  it('renders the OpenLux-style trigger with the Model placeholder when nothing is picked', () => {
    const wrapper = mountSelector({
      variant: 'select',
      options: ['claude-sonnet-5', 'gpt-5.3-codex']
    })

    const trigger = wrapper.findAll('button')[0]
    expect(trigger.text()).toBe('keys.modelPickerPlaceholder')
    // The panel stays closed until the trigger is clicked.
    expect(wrapper.find('input[placeholder="keys.modelPickerSearch"]').exists()).toBe(false)
  })

  it('summarises the selection in the trigger', () => {
    const wrapper = mountSelector({
      variant: 'select',
      modelValue: ['claude-sonnet-5', 'gpt-5.3-codex'],
      options: ['claude-sonnet-5', 'gpt-5.3-codex']
    })

    expect(wrapper.findAll('button')[0].text()).toBe('keys.modelPickerSelected')
  })

  it('opens a searchable list and toggles a model on click', async () => {
    const wrapper = mountSelector({
      variant: 'select',
      options: ['claude-sonnet-5', 'gpt-5.3-codex']
    })

    await wrapper.findAll('button')[0].trigger('click')
    await nextTick()

    const search = wrapper.find('input')
    expect(search.attributes('placeholder')).toBe('keys.modelPickerSearch')

    await optionButton(wrapper, 'gpt-5.3-codex')!.trigger('click')
    expect(lastEmitted(wrapper)).toEqual(['gpt-5.3-codex'])
  })

  it('filters the list by the search query', async () => {
    const wrapper = mountSelector({
      variant: 'select',
      options: ['claude-sonnet-5', 'gpt-5.3-codex']
    })

    await wrapper.findAll('button')[0].trigger('click')
    await nextTick()
    await wrapper.find('input').setValue('codex')
    await nextTick()

    expect(optionButton(wrapper, 'gpt-5.3-codex')).toBeTruthy()
    expect(optionButton(wrapper, 'claude-sonnet-5')).toBeUndefined()
  })

  it('fills a whole family with one preset click and clears it on the second', async () => {
    const wrapper = mountSelector({
      variant: 'select',
      options: ['claude-sonnet-5', 'gpt-5.3-codex'],
      presets: [
        { key: 'codex', label: 'Codex', models: ['gpt-5.3-codex'] },
        { key: 'anthropic', label: 'Anthropic', models: ['claude-sonnet-5'] }
      ]
    })

    await optionButton(wrapper, 'Codex')!.trigger('click')
    expect(lastEmitted(wrapper)).toEqual(['gpt-5.3-codex'])

    // Re-mount with the preset applied so the active state is real.
    const active = mountSelector({
      variant: 'select',
      modelValue: ['gpt-5.3-codex'],
      options: ['claude-sonnet-5', 'gpt-5.3-codex'],
      presets: [{ key: 'codex', label: 'Codex', models: ['gpt-5.3-codex'] }]
    })
    await optionButton(active, 'Codex')!.trigger('click')
    expect(lastEmitted(active)).toEqual([])
  })

  it('adds the preset models without dropping an unrelated pick', async () => {
    const wrapper = mountSelector({
      variant: 'select',
      modelValue: ['my-custom-model'],
      options: ['claude-sonnet-5', 'gpt-5.3-codex'],
      presets: [{ key: 'codex', label: 'Codex', models: ['gpt-5.3-codex'] }]
    })

    await optionButton(wrapper, 'Codex')!.trigger('click')
    expect(lastEmitted(wrapper)).toEqual(['my-custom-model', 'gpt-5.3-codex'])
  })

  it('keeps the chips layout for the account modals by default', () => {
    const wrapper = mountSelector({ options: ['claude-sonnet-5'] })
    // The default variant shows the chips container, never the select trigger.
    expect(wrapper.text()).toContain('admin.accounts.modelCount')
    expect(wrapper.text()).not.toContain('keys.modelPickerPlaceholder')
  })
})
