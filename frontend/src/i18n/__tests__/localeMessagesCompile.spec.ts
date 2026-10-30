import { describe, it, expect } from 'vitest'
import { baseCompile } from '@intlify/message-compiler'

import en from '../locales/en'
import ru from '../locales/ru'

/**
 * Every locale message is compiled by vue-i18n on first use, and this build uses
 * the message compiler in JIT mode (`__INTLIFY_JIT_COMPILATION__: true` in
 * `vite.config.ts`). A message the compiler cannot parse does NOT fall back to
 * the key — it THROWS, the exception escapes the render of the component that
 * translates the key, and (the app having no error boundary) the whole route is
 * left blank. That is how `/gateway/admin/settings` white-screened in production:
 *
 *   SyntaxError: Invalid linked format
 *     at parser (@intlify/message-compiler) <- JIT message compilation
 *     at render (SettingsView.vue:2154, t('admin.settings.telegram.channelId'))
 *     value: "-1001234567890 or @alerts_channel"
 *
 * In the message syntax `@` starts a LINKED message (`@:key`, `@.upper:key`) and
 * `|` separates PLURAL choices, so a literal one must be written as an
 * interpolation: `{'@'}` / `{'|'}`. Plain text like an e-mail address, a
 * Telegram handle or a `|` separator is not allowed raw.
 *
 * Both shipped locales are checked, because the active locale is what gets
 * compiled: a value broken only in `ru.ts` still white-screens a Russian user.
 */

const LOCALES: Array<[string, unknown]> = [
  ['en', en],
  ['ru', ru]
]

// A raw `|` only means "plural" where the message is written as one, e.g.
// "{count} new announcement | {count} new announcements".
const PLURAL_KEYS = new Set(['nav.newCount', 'newCount', 'announcements.newCount'])

type Leaf = { key: string; value: string }

function collect(node: unknown, prefix: string, out: Leaf[]): void {
  if (typeof node === 'string') {
    out.push({ key: prefix, value: node })
    return
  }
  if (Array.isArray(node)) {
    node.forEach((item, index) => collect(item, `${prefix}[${index}]`, out))
    return
  }
  if (node && typeof node === 'object') {
    for (const [key, value] of Object.entries(node as Record<string, unknown>)) {
      collect(value, prefix ? `${prefix}.${key}` : key, out)
    }
  }
}

function leaves(messages: unknown): Leaf[] {
  const out: Leaf[] = []
  collect(messages, '', out)
  return out
}

// Exactly the call the JIT path makes; `onError` throws instead of warning.
function compileMessage(value: string): unknown {
  return baseCompile(value, {
    jit: true,
    onError: (error: unknown) => {
      throw error
    }
  })
}

describe('locale messages compile', () => {
  for (const [locale, messages] of LOCALES) {
    it(`${locale}: every message is valid for the JIT compiler`, () => {
      const failures: string[] = []

      for (const { key, value } of leaves(messages)) {
        try {
          compileMessage(value)
        } catch (error) {
          failures.push(`${key}: ${(error as Error).message} — ${JSON.stringify(value)}`)
        }
      }

      expect(failures).toEqual([])
    })

    it(`${locale}: a literal @ or | is escaped as {'@'} / {'|'}`, () => {
      const failures: string[] = []

      for (const { key, value } of leaves(messages)) {
        // Drop interpolations (`{count}`, `{'@'}`) — inside them a special
        // character is a value, not message syntax.
        const literal = value.replace(/\{[^{}]*\}/g, '')
        if (!/[@|]/.test(literal)) {
          continue
        }
        if (PLURAL_KEYS.has(key) && !literal.includes('@')) {
          continue
        }
        const char = literal.includes('@') ? '@' : '|'
        failures.push(
          `${key}: raw "${char}" in message text — write it as {'${char}'} — ${JSON.stringify(value)}`
        )
      }

      expect(failures).toEqual([])
    })
  }
})
