import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getBillingGroups } = vi.hoisted(() => ({ getBillingGroups: vi.fn() }))
vi.mock('@/api/admin/providers', () => ({ getBillingGroups }))

import { useProviderBillingGroups } from '../useProviderBillingGroups'

describe('useProviderBillingGroups', () => {
  beforeEach(() => {
    getBillingGroups.mockReset()
  })

  it('maps the provider groups into labelled options (name · multiplier)', async () => {
    getBillingGroups.mockResolvedValue({
      provider: 'openlux',
      groups: [
        { name: 'Azure-Gpt-1', ratio: 0.044, models: 40 },
        { name: 'default', ratio: 1, models: 0 }
      ]
    })

    const groups = useProviderBillingGroups()
    await groups.load('openlux')

    expect(getBillingGroups).toHaveBeenCalledWith({ provider: 'openlux', base_url: undefined })
    expect(groups.options.value).toEqual([
      { value: 'Azure-Gpt-1', label: 'Azure-Gpt-1 · 0.044x', ratio: 0.044, models: 40 },
      { value: 'default', label: 'default · 1x', ratio: 1, models: 0 }
    ])
    expect(groups.loading.value).toBe(false)
  })

  it('keeps a group that is already stored but no longer advertised', async () => {
    getBillingGroups.mockResolvedValue({ provider: 'openlux', groups: [{ name: 'default', ratio: 1, models: 0 }] })

    const groups = useProviderBillingGroups()
    await groups.load('openlux')

    expect(groups.optionsWith('').map((o) => o.value)).toEqual(['default'])
    expect(groups.optionsWith('Kiro-Claude-1').map((o) => o.value)).toEqual(['Kiro-Claude-1', 'default'])
  })

  it('offers no presets when the provider cannot be read', async () => {
    getBillingGroups.mockRejectedValue(new Error('unreachable'))

    const groups = useProviderBillingGroups()
    await groups.load('openlux')

    expect(groups.options.value).toEqual([])
    expect(groups.loading.value).toBe(false)
  })

  it('fetches a provider once and reads a custom endpoint through its base url', async () => {
    getBillingGroups.mockResolvedValue({ provider: 'custom', groups: [] })

    const groups = useProviderBillingGroups()
    await groups.load('custom', 'https://example.com/v1')
    await groups.load('custom', 'https://example.com/v1')

    expect(getBillingGroups).toHaveBeenCalledTimes(1)
    expect(getBillingGroups).toHaveBeenCalledWith({ provider: 'custom', base_url: 'https://example.com/v1' })
  })
})
