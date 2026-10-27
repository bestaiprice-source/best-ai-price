import { describe, expect, it } from 'vitest'

import { buildApiSnippet, SNIPPET_MODELS } from '../apiSnippets'

const KEY = 'sk-test-key'
const BASE = 'https://api.example.com/v1'

describe('buildApiSnippet', () => {
  it('builds an OpenAI-compatible example per language, key inserted', () => {
    const python = buildApiSnippet({ protocol: 'openai', language: 'python', baseURL: BASE, apiKey: KEY })
    expect(python).toContain(`api_key="${KEY}"`)
    expect(python).toContain(`base_url="${BASE}"`)
    expect(python).toContain(`${BASE}/chat/completions`)
    expect(python).toContain(SNIPPET_MODELS.openai)

    const node = buildApiSnippet({ protocol: 'openai', language: 'nodejs', baseURL: BASE, apiKey: KEY })
    expect(node).toContain(`apiKey: '${KEY}'`)
    expect(node).toContain(`baseURL: '${BASE}'`)

    const curl = buildApiSnippet({ protocol: 'openai', language: 'curl', baseURL: BASE, apiKey: KEY })
    expect(curl).toContain(`curl ${BASE}/chat/completions`)
    expect(curl).toContain(`Authorization: Bearer ${KEY}`)
  })

  it('targets the native endpoint of the Anthropic and Gemini protocols', () => {
    const anthropic = buildApiSnippet({
      protocol: 'anthropic',
      language: 'curl',
      baseURL: 'https://api.example.com',
      apiKey: KEY
    })
    expect(anthropic).toContain('https://api.example.com/v1/messages')
    expect(anthropic).toContain(`x-api-key: ${KEY}`)
    expect(anthropic).toContain(SNIPPET_MODELS.anthropic)

    const gemini = buildApiSnippet({
      protocol: 'gemini',
      language: 'nodejs',
      baseURL: 'https://api.example.com',
      apiKey: KEY
    })
    expect(gemini).toContain(`/v1beta/models/${SNIPPET_MODELS.gemini}:generateContent`)
    expect(gemini).toContain(`apiKey: '${KEY}'`)
  })

  it('never leaves the prompt out of a snippet', () => {
    for (const protocol of ['openai', 'anthropic', 'gemini'] as const) {
      for (const language of ['python', 'nodejs', 'curl'] as const) {
        const snippet = buildApiSnippet({ protocol, language, baseURL: BASE, apiKey: KEY })
        expect(snippet).toContain('Hello, please introduce yourself in one sentence.')
      }
    }
  })
})
