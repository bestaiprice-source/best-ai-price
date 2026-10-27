/**
 * Ready-to-run API snippets shown to the user (API-key dialog, dashboard
 * onboarding). ONE generator for every surface, so the two screens can never
 * drift apart: the language tabs and the endpoint per protocol are decided here.
 */

export type ApiSnippetProtocol = 'openai' | 'anthropic' | 'gemini'
export type ApiSnippetLanguage = 'python' | 'nodejs' | 'curl'

export interface ApiSnippetOptions {
  protocol: ApiSnippetProtocol
  language: ApiSnippetLanguage
  /** Base URL a client of this protocol should point at (OpenAI gets the `/v1` suffix). */
  baseURL: string
  /** The caller's key, already resolved. */
  apiKey: string
}

/** The prompt every example sends, so the snippets read the same everywhere. */
export const SNIPPET_PROMPT = 'Hello, please introduce yourself in one sentence.'

/** One model per protocol that the gateway is guaranteed to price. */
export const SNIPPET_MODELS = {
  openai: 'gpt-5.2-pro',
  anthropic: 'claude-sonnet-4-5',
  gemini: 'gemini-2.5-flash'
} as const

export function buildApiSnippet({ protocol, language, baseURL, apiKey }: ApiSnippetOptions): string {
  const prompt = SNIPPET_PROMPT
  const openaiModel = SNIPPET_MODELS.openai
  const anthropicModel = SNIPPET_MODELS.anthropic
  const geminiModel = SNIPPET_MODELS.gemini
  const key = apiKey
  const base = baseURL
  if (protocol === 'openai') {
    if (language === 'python') {
      return `# POST ${base}/chat/completions (OpenAI compatible)
from openai import OpenAI

client = OpenAI(api_key="${key}", base_url="${base}")

resp = client.chat.completions.create(
    model="${openaiModel}",
    messages=[{"role": "user", "content": "${prompt}"}],
    temperature=0.7,
)
print(resp.choices[0].message.content)`
    }
    if (language === 'nodejs') {
      return `// POST ${base}/chat/completions (OpenAI compatible)
import OpenAI from 'openai'

const client = new OpenAI({ apiKey: '${key}', baseURL: '${base}' })

const resp = await client.chat.completions.create({
  model: '${openaiModel}',
  messages: [{ role: 'user', content: '${prompt}' }],
  temperature: 0.7,
})
console.log(resp.choices[0].message.content)`
    }
    return `curl ${base}/chat/completions \\
  -H "Authorization: Bearer ${key}" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"${openaiModel}","messages":[{"role":"user","content":"${prompt}"}],"temperature":0.7}'`
  }

  if (protocol === 'anthropic') {
    if (language === 'python') {
      return `# POST ${base}/v1/messages (Anthropic compatible)
from anthropic import Anthropic

client = Anthropic(api_key="${key}", base_url="${base}")

msg = client.messages.create(
    model="${anthropicModel}",
    max_tokens=256,
    messages=[{"role": "user", "content": "${prompt}"}],
)
print(msg.content[0].text)`
    }
    if (language === 'nodejs') {
      return `// POST ${base}/v1/messages (Anthropic compatible)
import Anthropic from '@anthropic-ai/sdk'

const client = new Anthropic({ apiKey: '${key}', baseURL: '${base}' })

const msg = await client.messages.create({
  model: '${anthropicModel}',
  max_tokens: 256,
  messages: [{ role: 'user', content: '${prompt}' }],
})
console.log(msg.content[0].text)`
    }
    return `curl ${base}/v1/messages \\
  -H "x-api-key: ${key}" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"${anthropicModel}","max_tokens":256,"messages":[{"role":"user","content":"${prompt}"}]}'`
  }

  // Gemini (native v1beta)
  if (language === 'python') {
    return `# POST ${base}/v1beta/models/${geminiModel}:generateContent (Gemini compatible)
from google import genai

client = genai.Client(api_key="${key}", http_options={"base_url": "${base}"})

resp = client.models.generate_content(
    model="${geminiModel}",
    contents="${prompt}",
)
print(resp.text)`
  }
  if (language === 'nodejs') {
    return `// POST ${base}/v1beta/models/${geminiModel}:generateContent (Gemini compatible)
import { GoogleGenAI } from '@google/genai'

const ai = new GoogleGenAI({ apiKey: '${key}', httpOptions: { baseUrl: '${base}' } })

const resp = await ai.models.generateContent({
  model: '${geminiModel}',
  contents: '${prompt}',
})
console.log(resp.text)`
  }
  return `curl "${base}/v1beta/models/${geminiModel}:generateContent" \\
  -H "x-goog-api-key: ${key}" \\
  -H "Content-Type: application/json" \\
  -d '{"contents":[{"parts":[{"text":"${prompt}"}]}]}'`
}
