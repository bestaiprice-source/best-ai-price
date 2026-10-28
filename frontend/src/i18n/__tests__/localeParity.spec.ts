import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import ru from '../locales/ru'

/**
 * Two locales ship (English + Russian) and the Russian file is derived from the
 * English one, so a key that exists in only one of them is a visible bug: the
 * Russian UI would render a raw dotted key path (`keys.title`) or silently fall
 * back to English mid-sentence. These guards keep the two trees in lockstep and
 * catch a translation that dropped a `{placeholder}`.
 */

type Tree = Record<string, unknown>

function leafPaths(node: unknown, prefix = ''): string[] {
  if (node === null || typeof node !== 'object') {
    return [prefix]
  }
  return Object.entries(node as Tree).flatMap(([key, value]) =>
    leafPaths(value, prefix ? `${prefix}.${key}` : key)
  )
}

function valueAt(tree: Tree, path: string): string {
  return path.split('.').reduce<unknown>((acc, key) => (acc as Tree)?.[key], tree) as string
}

const enPaths = leafPaths(en)
const ruPaths = leafPaths(ru)
const placeholders = (value: string) => (value.match(/\$\{[^}]+\}|\{[^}]+\}|%[sd]/g) ?? []).sort()

describe('locale parity (en ↔ ru)', () => {
  it('defines every English leaf key in Russian', () => {
    // Guard against a broken walker silently producing an empty comparison.
    expect(enPaths.length).toBeGreaterThan(4000)

    const missing = enPaths.filter((path) => !ruPaths.includes(path))
    expect(missing.slice(0, 20)).toEqual([])
    expect(missing).toHaveLength(0)
  })

  it('defines no Russian key that English does not have', () => {
    const extra = ruPaths.filter((path) => !enPaths.includes(path))
    expect(extra.slice(0, 20)).toEqual([])
    expect(extra).toHaveLength(0)
  })

  it('keeps every interpolation placeholder identical', () => {
    const drifted = enPaths.filter((path) => {
      const source = valueAt(en as Tree, path)
      const target = valueAt(ru as Tree, path)
      return placeholders(source).join(',') !== placeholders(target).join(',')
    })

    expect(drifted.slice(0, 20)).toEqual([])
    expect(drifted).toHaveLength(0)
  })

  it('is actually translated, not an English copy', () => {
    const identical = enPaths.filter(
      (path) => valueAt(en as Tree, path) === valueAt(ru as Tree, path)
    )

    // Names, ids and single words legitimately stay identical; a copy-pasted
    // English file would be ~100% identical.
    expect(identical.length / enPaths.length).toBeLessThan(0.2)
  })

  it('contains no CJK characters', () => {
    const withCjk = ruPaths.filter((path) =>
      /[\u4e00-\u9fff\u3040-\u30ff]/.test(valueAt(ru as Tree, path))
    )

    expect(withCjk.slice(0, 20)).toEqual([])
  })
})
