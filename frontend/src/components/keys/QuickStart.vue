<template>
  <div class="quick-start">
    <div class="flex items-center justify-between">
      <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('keys.quickStart.title') }}</h4>
      <span v-if="moreKeys > 0" class="text-[11px] text-gray-400">
        {{ t('keys.quickStart.moreKeys', { count: moreKeys }) }}
      </span>
    </div>

    <!-- Protocol -->
    <div class="mt-3 flex flex-wrap gap-4">
      <label
        v-for="option in protocols"
        :key="option.id"
        class="flex cursor-pointer items-center gap-2 text-sm text-gray-700 dark:text-gray-300"
      >
        <input
          v-model="activeProtocol"
          type="radio"
          :value="option.id"
          class="h-4 w-4 border-gray-300 text-primary-600 focus:ring-primary-500"
        />
        {{ option.label }}
      </label>
    </div>

    <!-- Base URL -->
    <div class="mt-3 flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-dark-600 dark:bg-dark-800">
      <code class="min-w-0 flex-1 truncate font-mono text-xs text-gray-700 dark:text-gray-300">{{ baseURL }}</code>
      <button type="button" class="shrink-0 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" :title="t('keys.quickStart.copy')" @click="copy(baseURL)">
        <Icon name="clipboard" size="sm" />
      </button>
    </div>

    <!-- Language tabs -->
    <div class="mt-4 flex gap-1 border-b border-gray-200 dark:border-dark-600">
      <button
        v-for="lang in languages"
        :key="lang.id"
        type="button"
        class="rounded-t-lg px-3 py-1.5 text-xs font-medium transition-colors"
        :class="activeLanguage === lang.id
          ? 'bg-gray-100 text-gray-900 dark:bg-dark-700 dark:text-white'
          : 'text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'"
        @click="activeLanguage = lang.id"
      >
        {{ lang.label }}
      </button>
    </div>

    <div class="relative mt-2">
      <pre class="max-h-72 overflow-auto rounded-lg bg-gray-900 p-3 pr-12 text-[11px] leading-relaxed text-gray-100"><code>{{ snippet }}</code></pre>
      <button
        type="button"
        class="absolute right-2 top-2 rounded-md bg-white/10 p-1.5 text-gray-200 transition-colors hover:bg-white/20"
        :title="t('keys.quickStart.copy')"
        @click="copy(snippet)"
      >
        <Icon :name="copied === 'snippet' ? 'check' : 'clipboard'" size="sm" />
      </button>
    </div>
    <p class="mt-2 text-[11px] text-gray-400">{{ t('keys.quickStart.keyInserted') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { useClipboard } from '@/composables/useClipboard'

const props = defineProps<{
  /** The key to embed in the snippets. */
  apiKey: string
  /** How many further keys were created in the same batch. */
  moreKeys?: number
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const activeProtocol = ref<'openai' | 'anthropic' | 'gemini'>('openai')
const activeLanguage = ref<'python' | 'nodejs' | 'curl'>('python')
const copied = ref('')

const protocols = computed(() => [
  { id: 'openai' as const, label: 'OpenAI' },
  { id: 'anthropic' as const, label: 'Anthropic' },
  { id: 'gemini' as const, label: 'Gemini' },
])

const languages = computed(() => [
  { id: 'python' as const, label: 'Python' },
  { id: 'nodejs' as const, label: 'Node.js' },
  { id: 'curl' as const, label: 'cURL' },
])

const moreKeys = computed(() => props.moreKeys ?? 0)

/** Public gateway base URL, falling back to the current origin. */
const gatewayBase = computed(() => {
  const configured = String(appStore.cachedPublicSettings?.api_base_url || '').trim()
  return (configured || window.location.origin).replace(/\/+$/, '')
})

/** Base URL a client of the selected protocol should point at. */
const baseURL = computed(() => (activeProtocol.value === 'openai' ? `${gatewayBase.value}/v1` : gatewayBase.value))

const prompt = 'Hello, please introduce yourself in one sentence.'
const openaiModel = 'gpt-5.2-pro'
const anthropicModel = 'claude-sonnet-4-5'
const geminiModel = 'gemini-2.5-flash'

const snippet = computed(() => {
  const key = props.apiKey
  const base = baseURL.value
  if (activeProtocol.value === 'openai') {
    if (activeLanguage.value === 'python') {
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
    if (activeLanguage.value === 'nodejs') {
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

  if (activeProtocol.value === 'anthropic') {
    if (activeLanguage.value === 'python') {
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
    if (activeLanguage.value === 'nodejs') {
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
  if (activeLanguage.value === 'python') {
    return `# POST ${base}/v1beta/models/${geminiModel}:generateContent (Gemini compatible)
from google import genai

client = genai.Client(api_key="${key}", http_options={"base_url": "${base}"})

resp = client.models.generate_content(
    model="${geminiModel}",
    contents="${prompt}",
)
print(resp.text)`
  }
  if (activeLanguage.value === 'nodejs') {
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
})

async function copy(value: string) {
  if (!value) return
  const ok = await copyToClipboard(value)
  if (ok !== false) {
    copied.value = value === baseURL.value ? 'base' : 'snippet'
    setTimeout(() => (copied.value = ''), 1500)
  }
}
</script>