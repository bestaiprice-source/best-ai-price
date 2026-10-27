import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import en from '../locales/en'

/**
 * The usage screens rendered raw dotted key paths (`usage.tabRequests`,
 * `USAGE.REQUESTID`, …) because the Errors tab and the Request ID column asked
 * for `usage.*` keys that only existed under `dashboard.*`. A key the view
 * references but the locale does not define is a visible bug, so assert the two
 * sets line up.
 */
const VIEWS = ['src/views/user/UsageView.vue', 'src/views/admin/UsageView.vue']

function referencedUsageKeys(): string[] {
  const keys = new Set<string>()
  for (const view of VIEWS) {
    const source = readFileSync(resolve(process.cwd(), view), 'utf8')
    for (const literal of source.match(/t\('usage\.[A-Za-z0-9_]+'/g) ?? []) {
      keys.add(literal.slice("t('usage.".length, -1))
    }
  }
  return Array.from(keys).sort()
}

describe('usage locale keys', () => {
  it('defines every usage.* key the usage screens reference', () => {
    const keys = referencedUsageKeys()
    // Guard against the regex silently matching nothing.
    expect(keys.length).toBeGreaterThan(40)

    const enUsage = en.usage as Record<string, unknown>

    expect(keys.filter((key) => !(key in enUsage))).toEqual([])
  })
})
