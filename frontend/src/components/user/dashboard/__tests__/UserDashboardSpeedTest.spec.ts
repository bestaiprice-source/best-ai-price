import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

vi.mock('@/api/publicModels', () => ({
  getPublicModels: vi.fn().mockResolvedValue([
    { canonical_model_id: 'anthropic/claude-sonnet-5', model_id: 'claude-sonnet-5', input_price_per_1m: 0.19 },
    // Unpriced: the speed test must never be pointed at it.
    { canonical_model_id: 'openai/gpt-unpriced', model_id: 'gpt-unpriced' }
  ])
}))

vi.mock('@/api/speedTest', () => ({
  runSpeedTest: vi.fn()
}))

import { runSpeedTest } from '@/api/speedTest'
import UserDashboardSpeedTest from '../UserDashboardSpeedTest.vue'

const mockedRun = vi.mocked(runSpeedTest)

const mountCard = async () => {
  const wrapper = mount(UserDashboardSpeedTest)
  await flushPromises()
  return wrapper
}

describe('UserDashboardSpeedTest', () => {
  beforeEach(() => {
    mockedRun.mockReset()
    localStorage.clear()
  })

  it('offers only models that have a published price', async () => {
    const wrapper = await mountCard()
    const options = wrapper.findAll('option').map((o) => o.text())
    expect(options).toEqual(['claude-sonnet-5'])
  })

  it('measures on demand and shows the time to first token', async () => {
    mockedRun.mockResolvedValue({
      model: 'claude-sonnet-5',
      ttft_ms: 842,
      total_ms: 1180,
      input_tokens: 12,
      output_tokens: 1,
      node_id: 'node-abcdef123456'
    })
    const wrapper = await mountCard()

    expect(wrapper.text()).toContain('speedTest.idle')
    await wrapper.find('button').trigger('click')
    await flushPromises()

    expect(mockedRun).toHaveBeenCalledWith('claude-sonnet-5')
    expect(wrapper.text()).toContain('842 ms')
    expect(wrapper.text()).toContain('1180 ms')
    // The last measurement survives a reload.
    expect(localStorage.getItem('sub2api_speed_test')).toContain('842')
  })

  it('surfaces a failure instead of silently doing nothing', async () => {
    mockedRun.mockRejectedValue(new Error('Create an API key first'))
    const wrapper = await mountCard()
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Create an API key first')
  })

  it('restores the previous measurement from storage', async () => {
    localStorage.setItem(
      'sub2api_speed_test',
      JSON.stringify({
        at: Date.now(),
        result: { model: 'claude-sonnet-5', ttft_ms: 611, total_ms: 900, input_tokens: 9, output_tokens: 1, node_id: 'n1' }
      })
    )
    const wrapper = await mountCard()
    expect(wrapper.text()).toContain('611 ms')
    expect(wrapper.text()).not.toContain('speedTest.idle')
  })
})
