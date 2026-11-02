import { describe, expect, it } from 'vitest'
import {
  ALLOWLIST_PRESETS,
  buildAllowlistPresets,
  modelsForPreset,
  stripModelVendorPrefix
} from '../useModelAllowlistPresets'

const preset = (key: string) => ALLOWLIST_PRESETS.find((p) => p.key === key)!

describe('useModelAllowlistPresets', () => {
  it('strips a known vendor prefix and keeps an unknown one', () => {
    expect(stripModelVendorPrefix('anthropic/claude-sonnet-5')).toBe('claude-sonnet-5')
    expect(stripModelVendorPrefix('claude-sonnet-5')).toBe('claude-sonnet-5')
    // Not a vendor prefix the backend strips -> the id stays whole.
    expect(stripModelVendorPrefix('some/custom-model')).toBe('some/custom-model')
  })

  it('selects only the family members of the pool', () => {
    const pool = [
      'anthropic/claude-sonnet-5',
      'google/gemini-2.5-flash',
      'openai/gpt-5.3-codex',
      'gpt-4o'
    ]

    expect(modelsForPreset(preset('codex'), pool)).toContain('openai/gpt-5.3-codex')
    expect(modelsForPreset(preset('anthropic'), pool)).toContain('anthropic/claude-sonnet-5')
    expect(modelsForPreset(preset('gemini'), pool)).toContain('google/gemini-2.5-flash')

    // No cross-family leakage from the pool.
    expect(modelsForPreset(preset('codex'), pool)).not.toContain('anthropic/claude-sonnet-5')
    expect(modelsForPreset(preset('anthropic'), pool)).not.toContain('openai/gpt-5.3-codex')
    expect(modelsForPreset(preset('gemini'), pool)).not.toContain('gpt-4o')
  })

  it('falls back to the curated family list so a preset is never empty', () => {
    // The Codex family is not in the pooled-provider curated list, so the pool
    // cannot supply it — the curated OpenAI list must.
    const codex = modelsForPreset(preset('codex'), [])
    expect(codex.length).toBeGreaterThan(0)
    expect(codex.every((id) => id.includes('codex'))).toBe(true)
  })

  it('builds labelled presets over the pool', () => {
    const built = buildAllowlistPresets(['anthropic/claude-sonnet-5'], (key) => `t:${key}`)
    expect(built.map((p) => p.key)).toEqual(['codex', 'anthropic', 'gemini'])
    expect(built.find((p) => p.key === 'anthropic')!.label).toBe('t:keys.modelPresets.anthropic')
    expect(built.find((p) => p.key === 'anthropic')!.models).toContain('anthropic/claude-sonnet-5')
  })
})
