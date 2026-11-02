/**
 * Quick presets for the API-key model allowlist.
 *
 * A preset is a named family (Codex / Anthropic / Gemini) that fills the
 * allowlist with every model of that family we can actually serve. The family
 * list is drawn from the PICKER'S OWN option pool (which merges the live public
 * catalog with the curated family lists), so a preset can never contain a model
 * the dropdown does not offer or the gateway cannot route. The curated lists are
 * scanned as well, so a preset stays usable when the catalog could not be
 * fetched.
 *
 * The matcher mirrors the backend's model-restriction matching
 * (`stripModelVendorPrefix` in `internal/handler/api_key_model_restriction.go`):
 * an id is compared without its vendor prefix, so the catalog's
 * `anthropic/claude-sonnet-5` and a request for `claude-sonnet-5` land on the
 * same family.
 */

import { getModelsByPlatform } from './useModelWhitelist'

/** Vendor prefixes the backend strips before comparing an allowlist entry. */
const VENDOR_PREFIXES = [
  'anthropic', 'openai', 'google', 'deepseek', 'moonshot', 'meta-llama',
  'qwen', 'mistral', 'cohere', 'minimax', 'zhipu', 'xai'
]

/** Drops a leading vendor prefix (`anthropic/claude-sonnet-5` → `claude-sonnet-5`). */
export function stripModelVendorPrefix(id: string): string {
  const model = (id || '').trim().toLowerCase()
  const idx = model.indexOf('/')
  if (idx !== -1 && VENDOR_PREFIXES.includes(model.slice(0, idx))) {
    return model.slice(idx + 1)
  }
  return model
}

export type AllowlistPresetKey = 'codex' | 'anthropic' | 'gemini'

export interface AllowlistPresetDef {
  key: AllowlistPresetKey
  /** i18n key holding the button label. */
  labelKey: string
  /** Platform whose curated model list backs the preset offline. */
  fallbackPlatform: string
  /** Whether a model id belongs to this family (vendor prefix stripped). */
  matches: (id: string) => boolean
}

export const ALLOWLIST_PRESETS: AllowlistPresetDef[] = [
  {
    key: 'codex',
    labelKey: 'keys.modelPresets.codex',
    fallbackPlatform: 'openai',
    matches: (id) => stripModelVendorPrefix(id).includes('codex')
  },
  {
    key: 'anthropic',
    labelKey: 'keys.modelPresets.anthropic',
    fallbackPlatform: 'anthropic',
    matches: (id) => stripModelVendorPrefix(id).startsWith('claude')
  },
  {
    key: 'gemini',
    labelKey: 'keys.modelPresets.gemini',
    fallbackPlatform: 'gemini',
    matches: (id) => stripModelVendorPrefix(id).startsWith('gemini')
  }
]

/**
 * The models a preset fills in: every family member of `pool` (the picker's
 * option list) plus the curated family list, de-duplicated in that order so the
 * live catalog's ids win.
 */
export function modelsForPreset(preset: AllowlistPresetDef, pool: string[]): string[] {
  const out = new Set<string>()
  for (const id of pool) {
    if (preset.matches(id)) out.add(id)
  }
  for (const id of getModelsByPlatform(preset.fallbackPlatform)) {
    if (preset.matches(id)) out.add(id)
  }
  return Array.from(out)
}

/** Builds the render-ready preset list for a picker's option pool. */
export function buildAllowlistPresets(
  pool: string[],
  label: (key: string) => string
): { key: string; label: string; models: string[] }[] {
  return ALLOWLIST_PRESETS.map((preset) => ({
    key: preset.key,
    label: label(preset.labelKey),
    models: modelsForPreset(preset, pool)
  }))
}
