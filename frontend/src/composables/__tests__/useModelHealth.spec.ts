import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/modelHealth', () => ({
  getModelHealth: vi.fn()
}))

import { getModelHealth } from '@/api/modelHealth'
import {
  modelLookupKeys,
  resetModelHealthCache,
  useModelHealth
} from '../useModelHealth'
import type { ModelHealthItem } from '@/api/modelHealth'

const item = (overrides: Partial<ModelHealthItem>): ModelHealthItem => ({
  model: 'm',
  candidates: ['m'],
  status: 'operational',
  availability_pct: 99.44,
  avg_latency_ms: 800,
  provider: 'anthropic',
  monitor_id: 1,
  monitor_name: 'OpenLux',
  maintenance_until: null,
  maintenance_note: '',
  ...overrides
})

const mockedGet = vi.mocked(getModelHealth)

describe('modelLookupKeys', () => {
  it('tries the canonical id and its vendor-less slug', () => {
    expect(modelLookupKeys('anthropic/claude-3-7-sonnet')).toEqual([
      'anthropic/claude-3-7-sonnet',
      'claude-3-7-sonnet'
    ])
  })

  it('normalizes case and blanks', () => {
    expect(modelLookupKeys('  OpenAI/GPT-5.2  ')).toEqual(['openai/gpt-5.2', 'gpt-5.2'])
    expect(modelLookupKeys('')).toEqual([])
    expect(modelLookupKeys(null)).toEqual([])
  })
})

describe('useModelHealth join', () => {
  beforeEach(() => {
    resetModelHealthCache()
    mockedGet.mockReset()
  })

  it('joins a catalog model onto the monitor entry through the shared keys', async () => {
    mockedGet.mockResolvedValue({
      window_days: 7,
      items: [
        item({
          model: 'claude-3-7-sonnet-20250219',
          candidates: ['claude-3-7-sonnet-20250219', 'claude-3-7-sonnet'],
          status: 'operational',
          availability_pct: 99.44
        })
      ]
    })
    const { load, resolve } = useModelHealth()
    await load()

    expect(resolve('anthropic/claude-3-7-sonnet')?.availability_pct).toBe(99.44)
    expect(resolve('claude-3-7-sonnet')?.status).toBe('operational')
  })

  it('returns null for a model no monitor covers', async () => {
    mockedGet.mockResolvedValue({ window_days: 7, items: [item({})] })
    const { load, resolve } = useModelHealth()
    await load()
    expect(resolve('openai/gpt-5.2')).toBeNull()
  })

  it('prefers the healthiest match when several monitors cover one model', async () => {
    mockedGet.mockResolvedValue({
      window_days: 7,
      items: [
        item({ monitor_id: 1, status: 'failed', availability_pct: 12, candidates: ['gpt-5.2'] }),
        item({ monitor_id: 2, status: 'operational', availability_pct: 99.9, candidates: ['gpt-5.2'] })
      ]
    })
    const { load, resolve } = useModelHealth()
    await load()
    // One failing provider must not paint a model red while another serves it.
    expect(resolve('openai/gpt-5.2')?.status).toBe('operational')
  })

  it('shares one request between concurrent callers and caches the result', async () => {
    mockedGet.mockResolvedValue({ window_days: 7, items: [item({})] })
    const { load } = useModelHealth()
    await Promise.all([load(), load()])
    await load()
    expect(mockedGet).toHaveBeenCalledTimes(1)
  })

  it('surfaces a failure as an empty map instead of throwing', async () => {
    mockedGet.mockRejectedValue(new Error('boom'))
    const { load, error } = useModelHealth()
    await expect(load()).resolves.toBeUndefined()
    expect(error.value).toBe('boom')
  })

  it('re-fetches when forced', async () => {
    mockedGet.mockResolvedValue({ window_days: 7, items: [] })
    const { load } = useModelHealth()
    await load()
    await load(true)
    expect(mockedGet).toHaveBeenCalledTimes(2)
  })
})
