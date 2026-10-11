<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-8 pb-16">
      <!-- Header -->
      <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-3xl font-bold tracking-tight text-gray-900 dark:text-white">{{ t('docs.title') }}</h1>
          <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('docs.subtitle') }}</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <a
            href="https://github.com/bestaiprice/api-examples"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex items-center gap-2 rounded-xl bg-gray-900 px-4 py-2 text-xs font-semibold text-white shadow-sm transition hover:bg-gray-800 dark:bg-dark-700 dark:hover:bg-dark-600"
          >
            <span>{{ t('docs.banner.githubLink') }}</span>
            <span class="text-xs opacity-75">↗</span>
          </a>
        </div>
      </div>

      <!-- GitHub Developer Solutions Banner (§14) -->
      <section class="rounded-2xl border border-primary-200/80 bg-gradient-to-r from-primary-50/70 via-white to-blue-50/50 p-6 shadow-sm dark:border-primary-900/40 dark:from-primary-950/20 dark:via-dark-800 dark:to-blue-950/20">
        <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div class="space-y-1">
            <div class="inline-flex items-center gap-1.5 rounded-full bg-primary-100/80 px-2.5 py-0.5 text-[11px] font-semibold text-primary-800 dark:bg-primary-900/50 dark:text-primary-300">
              <span>{{ t('docs.banner.badge') }}</span>
            </div>
            <h2 class="text-base font-bold text-gray-900 dark:text-white">{{ t('docs.banner.title') }}</h2>
            <p class="text-xs text-gray-600 dark:text-gray-300">
              {{ t('docs.banner.subtitle') }}
            </p>
          </div>
          <div class="flex flex-shrink-0 items-center gap-2">
            <button
              class="inline-flex items-center gap-1.5 rounded-lg border border-gray-200 bg-white px-3 py-2 text-xs font-medium text-gray-700 shadow-sm transition hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200"
              @click="copy('git clone https://github.com/bestaiprice/api-examples.git')"
            >
              <span>{{ copied === 'git clone https://github.com/bestaiprice/api-examples.git' ? t('docs.copied') : t('docs.banner.copyClone') }}</span>
            </button>
          </div>
        </div>
      </section>

      <!-- Quick start -->
      <section class="rounded-2xl border border-gray-200/80 bg-white/85 p-6 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('docs.quickStart.title') }}</h2>
        <ol class="mt-4 space-y-4">
          <li class="flex gap-3">
            <span class="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-950/50 dark:text-primary-300">1</span>
            <div>
              <p class="font-medium text-gray-900 dark:text-white">{{ t('docs.quickStart.step1Title') }}</p>
              <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('docs.quickStart.step1Body') }}</p>
              <router-link to="/purchase" class="mt-1 inline-block text-sm font-medium text-primary-600 hover:underline">{{ t('docs.quickStart.step1Cta') }}</router-link>
            </div>
          </li>
          <li class="flex gap-3">
            <span class="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-950/50 dark:text-primary-300">2</span>
            <div>
              <p class="font-medium text-gray-900 dark:text-white">{{ t('docs.quickStart.step2Title') }}</p>
              <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('docs.quickStart.step2Body') }}</p>
              <router-link to="/keys" class="mt-1 inline-block text-sm font-medium text-primary-600 hover:underline">{{ t('docs.quickStart.step2Cta') }}</router-link>
            </div>
          </li>
          <li class="flex gap-3">
            <span class="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-950/50 dark:text-primary-300">3</span>
            <div>
              <p class="font-medium text-gray-900 dark:text-white">{{ t('docs.quickStart.step3Title') }}</p>
              <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('docs.quickStart.step3Body') }}</p>
              <router-link to="/available-channels" class="mt-1 inline-block text-sm font-medium text-primary-600 hover:underline">{{ t('docs.quickStart.step3Cta') }}</router-link>
            </div>
          </li>
        </ol>
      </section>

      <!-- Base URL & auth -->
      <section class="rounded-2xl border border-gray-200/80 bg-white/85 p-6 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('docs.auth.title') }}</h2>
        <div class="mt-4 space-y-3 text-sm">
          <div>
            <p class="text-gray-500 dark:text-gray-400">{{ t('docs.auth.baseUrl') }}</p>
            <div class="mt-1 flex items-center gap-2">
              <code class="flex-1 rounded-lg bg-gray-50 px-3 py-2 font-mono text-xs text-gray-800 dark:bg-dark-900 dark:text-gray-200">{{ baseUrl }}</code>
              <button class="rounded-lg border border-gray-200 px-2 py-1.5 text-xs text-gray-500 hover:bg-gray-50 dark:border-dark-700 dark:text-gray-400" @click="copy(baseUrl)">
                {{ copied === baseUrl ? t('docs.copied') : t('docs.copy') }}
              </button>
            </div>
          </div>
          <div>
            <p class="text-gray-500 dark:text-gray-400">{{ t('docs.auth.header') }}</p>
            <div class="mt-1 flex items-center gap-2">
              <code class="flex-1 rounded-lg bg-gray-50 px-3 py-2 font-mono text-xs text-gray-800 dark:bg-dark-900 dark:text-gray-200">Authorization: Bearer YOUR_API_KEY</code>
              <button class="rounded-lg border border-gray-200 px-2 py-1.5 text-xs text-gray-500 hover:bg-gray-50 dark:border-dark-700 dark:text-gray-400" @click="copy('Authorization: Bearer YOUR_API_KEY')">
                {{ copied === 'Authorization: Bearer YOUR_API_KEY' ? t('docs.copied') : t('docs.copy') }}
              </button>
            </div>
            <p class="mt-1 text-xs text-gray-400">{{ t('docs.auth.note') }}</p>
          </div>
        </div>
      </section>

      <!-- Endpoints -->
      <section class="rounded-2xl border border-gray-200/80 bg-white/85 p-6 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('docs.endpoints.title') }}</h2>
        <div class="mt-4 overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
            <thead class="text-left text-xs uppercase tracking-wider text-gray-500 dark:text-gray-400">
              <tr>
                <th class="py-2 pr-4">{{ t('docs.endpoints.method') }}</th>
                <th class="py-2 pr-4">{{ t('docs.endpoints.path') }}</th>
                <th class="py-2">{{ t('docs.endpoints.description') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-50 dark:divide-dark-800">
              <tr v-for="ep in endpoints" :key="ep.path">
                <td class="py-2 pr-4"><span class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs dark:bg-dark-700">{{ ep.method }}</span></td>
                <td class="py-2 pr-4 font-mono text-xs text-gray-800 dark:text-gray-200">{{ ep.path }}</td>
                <td class="py-2 text-gray-500 dark:text-gray-400">{{ ep.desc }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <!-- Code examples -->
      <section class="rounded-2xl border border-gray-200/80 bg-white/85 p-6 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('docs.examples.title') }}</h2>
        <div class="mt-3 flex flex-wrap gap-2">
          <button
            v-for="tab in exampleTabs"
            :key="tab.id"
            class="rounded-full border px-3 py-1 text-xs font-medium transition-colors"
            :class="activeExample === tab.id
              ? 'border-primary-400 bg-primary-50 text-primary-700 dark:border-primary-700 dark:bg-primary-950/40 dark:text-primary-300'
              : 'border-gray-200 bg-white text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300'"
            @click="activeExample = tab.id"
          >
            {{ tab.label }}
          </button>
        </div>

        <div class="mt-4 space-y-4">
          <div v-for="snippet in activeSnippets" :key="snippet.label">
            <div class="flex items-center justify-between">
              <p class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ snippet.label }}</p>
              <button class="rounded-lg border border-gray-200 px-2 py-1 text-xs text-gray-500 hover:bg-gray-50 dark:border-dark-700 dark:text-gray-400" @click="copy(snippet.code)">
                {{ copied === snippet.code ? t('docs.copied') : t('docs.copy') }}
              </button>
            </div>
            <pre class="mt-1 overflow-x-auto rounded-xl bg-gray-900 p-4 text-xs leading-relaxed text-gray-100"><code>{{ snippet.code }}</code></pre>
          </div>
        </div>
      </section>

      <!-- Models -->
      <section class="rounded-2xl border border-gray-200/80 bg-white/85 p-6 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('docs.models.title') }}</h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('docs.models.body') }}</p>
        <div class="mt-3 flex items-center gap-2">
          <code class="flex-1 rounded-lg bg-gray-50 px-3 py-2 font-mono text-xs text-gray-800 dark:bg-dark-900 dark:text-gray-200">GET {{ baseUrl }}/v1/models</code>
          <button class="rounded-lg border border-gray-200 px-2 py-1.5 text-xs text-gray-500 hover:bg-gray-50 dark:border-dark-700 dark:text-gray-400" @click="copy('curl ' + baseUrl + '/v1/models -H &quot;Authorization: Bearer $YOUR_API_KEY&quot;')">
            {{ t('docs.copyCurl') }}
          </button>
        </div>
        <router-link to="/available-channels" class="mt-3 inline-block text-sm font-medium text-primary-600 hover:underline">{{ t('docs.models.openCatalog') }}</router-link>
      </section>

      <!-- Integrations -->
      <section class="rounded-2xl border border-gray-200/80 bg-white/85 p-6 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('docs.integrations.title') }}</h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('docs.integrations.body') }}</p>
        <div class="mt-4 space-y-4">
          <div v-for="intg in integrations" :key="intg.name" class="rounded-xl border border-gray-100 p-4 dark:border-dark-700">
            <div class="flex items-start justify-between gap-3">
              <div>
                <p class="font-medium text-gray-900 dark:text-white">{{ intg.name }}</p>
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ intg.desc }}</p>
              </div>
              <button class="flex-shrink-0 rounded-lg border border-gray-200 px-2 py-1 text-xs text-gray-500 hover:bg-gray-50 dark:border-dark-700 dark:text-gray-400" @click="copy(intg.config)">
                {{ copied === intg.config ? t('docs.copied') : t('docs.copy') }}
              </button>
            </div>
            <pre class="mt-2 overflow-x-auto rounded-lg bg-gray-900 p-3 text-xs text-gray-100"><code>{{ intg.config }}</code></pre>
          </div>
        </div>
      </section>

      <!-- FAQ Section (§15) -->
      <section class="rounded-2xl border border-gray-200/80 bg-white/85 p-6 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('docs.faq.title') }}</h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('docs.faq.subtitle') }}</p>
          </div>
        </div>
        <div class="mt-4 divide-y divide-gray-100 dark:divide-dark-700">
          <div v-for="(item, idx) in faqItems" :key="idx" class="py-3">
            <button
              class="flex w-full items-center justify-between text-left text-sm font-medium text-gray-800 dark:text-gray-200"
              @click="toggleFaq(idx)"
            >
              <span>{{ item.q }}</span>
              <span class="ml-2 font-mono text-gray-400">{{ expandedFaq === idx ? '−' : '+' }}</span>
            </button>
            <p v-if="expandedFaq === idx" class="mt-2 text-xs leading-relaxed text-gray-600 dark:text-gray-400">
              {{ item.a }}
            </p>
          </div>
        </div>
      </section>

      <!-- Support & Disputed Charges (§15) -->
      <section class="rounded-2xl border border-gray-200/80 bg-white/85 p-6 shadow-sm dark:border-dark-700/80 dark:bg-dark-800/85">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('support.dispute') }}</h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('support.disputeHint') }}</p>

        <div class="mt-4 space-y-3">
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <label class="block text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('support.requestIdLabel') }}</label>
              <input
                v-model="disputeRequestId"
                type="text"
                class="input mt-1 w-full text-xs"
                placeholder="req_01j7..."
              />
            </div>
            <div>
              <label class="block text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('support.modelNameLabel') }}</label>
              <input
                v-model="disputeModelName"
                type="text"
                class="input mt-1 w-full text-xs"
                :placeholder="t('support.modelNamePlaceholder')"
              />
            </div>
          </div>

          <div>
            <div class="flex items-center justify-between">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('support.sanitizedContext') }}</span>
              <button
                class="rounded-lg border border-gray-200 px-2 py-1 text-xs text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:text-gray-300"
                @click="copy(disputeContextText)"
              >
                {{ copied === disputeContextText ? t('docs.copied') : t('docs.copy') }}
              </button>
            </div>
            <pre class="mt-1 overflow-x-auto rounded-xl bg-gray-900 p-4 font-mono text-xs text-gray-100"><code>{{ disputeContextText }}</code></pre>
            <p class="mt-1.5 text-[11px] text-gray-400">{{ t('support.noSecrets') }}</p>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()

const copied = ref('')
const expandedFaq = ref<number | null>(0)
const disputeRequestId = ref('')
const disputeModelName = ref('')

function toggleFaq(idx: number) {
  expandedFaq.value = expandedFaq.value === idx ? null : idx
}

// The public API base URL, as configured in Admin Settings; falls back to the
// current origin so the examples are always copy-paste runnable.
const baseUrl = computed(() => {
  const raw = String(appStore.cachedPublicSettings?.api_base_url || appStore.apiBaseUrl || '').trim()
  const url = raw || window.location.origin
  return url.replace(/\/+$/, '')
})

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    copied.value = text
    setTimeout(() => {
      if (copied.value === text) copied.value = ''
    }, 1500)
  } catch (e) {
    console.warn('Copy failed:', e)
  }
}

const endpoints = computed(() => [
  { method: 'POST', path: '/v1/messages', desc: t('docs.endpoints.messages') },
  { method: 'POST', path: '/v1/chat/completions', desc: t('docs.endpoints.chat') },
  { method: 'POST', path: '/responses', desc: t('docs.endpoints.responses') },
  { method: 'POST', path: '/v1beta/…', desc: t('docs.endpoints.gemini') },
  { method: 'GET', path: '/v1/models', desc: t('docs.endpoints.models') },
])

const activeExample = ref('curl')
const exampleTabs = computed(() => [
  { id: 'curl', label: 'cURL' },
  { id: 'python', label: 'Python' },
  { id: 'node', label: 'Node.js' },
  { id: 'stream', label: 'Streaming (SSE)' },
  { id: 'gemini', label: 'Google Gemini' },
])

const activeSnippets = computed(() => {
  const base = baseUrl.value
  if (activeExample.value === 'python') {
    return [
      {
        label: 'OpenAI SDK',
        code: `from openai import OpenAI
import os

client = OpenAI(
    base_url="${base}/v1",
    api_key=os.environ.get("BESTAIPRICE_API_KEY"),
)
resp = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Hello BestAIPrice!"}],
)
print(resp.choices[0].message.content)`,
      },
      {
        label: 'Anthropic SDK',
        code: `from anthropic import Anthropic
import os

client = Anthropic(
    base_url="${base}",
    api_key=os.environ.get("BESTAIPRICE_API_KEY"),
)
msg = client.messages.create(
    model="claude-3-7-sonnet",
    max_tokens=256,
    messages=[{"role": "user", "content": "Hello Claude!"}],
)
print(msg.content[0].text)`,
      },
    ]
  }
  if (activeExample.value === 'node') {
    return [
      {
        label: 'Native fetch (Node.js 18+)',
        code: `const res = await fetch("${base}/v1/chat/completions", {
  method: "POST",
  headers: {
    "Authorization": \`Bearer \${process.env.BESTAIPRICE_API_KEY}\`,
    "content-type": "application/json",
  },
  body: JSON.stringify({
    model: "gpt-4o",
    messages: [{ role: "user", content: "Hello!" }],
  }),
});
console.log(await res.json());`,
      },
      {
        label: 'openai (npm)',
        code: `import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "${base}/v1",
  apiKey: process.env.BESTAIPRICE_API_KEY,
});

const resp = await client.chat.completions.create({
  model: "gpt-4o",
  messages: [{ role: "user", content: "Hello!" }],
});
console.log(resp.choices[0].message.content);`,
      },
    ]
  }
  if (activeExample.value === 'stream') {
    return [
      {
        label: 'cURL Unbuffered SSE Stream',
        code: `curl -N --no-buffer -X POST "${base}/v1/chat/completions" \\
  -H "Authorization: Bearer $BESTAIPRICE_API_KEY" \\
  -H "Content-Type: application/json" \\
  -H "Accept: text/event-stream" \\
  -d '{
    "model": "gpt-4o",
    "stream": true,
    "messages": [{"role": "user", "content": "Stream a short story"}]
  }'`,
      },
      {
        label: 'Python Token Stream',
        code: `from openai import OpenAI
import os, sys

client = OpenAI(base_url="${base}/v1", api_key=os.environ["BESTAIPRICE_API_KEY"])
stream = client.chat.completions.create(
    model="deepseek-chat",
    messages=[{"role": "user", "content": "Stream a story"}],
    stream=True,
)
for chunk in stream:
    if chunk.choices and chunk.choices[0].delta.content:
        sys.stdout.write(chunk.choices[0].delta.content)
        sys.stdout.flush()`,
      },
    ]
  }
  if (activeExample.value === 'gemini') {
    return [
      {
        label: 'Gemini v1beta REST API',
        code: `curl -X POST "${base}/v1beta/models/gemini-2.5-flash:generateContent" \\
  -H "x-goog-api-key: $BESTAIPRICE_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "contents": [
      { "parts": [{ "text": "Explain quantum computing in 1 sentence." }] }
    ]
  }'`,
      },
    ]
  }
  return [
    {
      label: 'Anthropic Messages API',
      code: `curl ${base}/v1/messages \\
  -H "x-api-key: $BESTAIPRICE_API_KEY" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "content-type: application/json" \\
  -d '{
    "model": "claude-3-7-sonnet",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": "Hello!"}]
  }'`,
    },
    {
      label: 'OpenAI Chat Completions API',
      code: `curl ${base}/v1/chat/completions \\
  -H "Authorization: Bearer $BESTAIPRICE_API_KEY" \\
  -H "content-type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'`,
    },
  ]
})

const integrations = computed(() => {
  const base = baseUrl.value
  return [
    {
      name: 'Claude Code CLI',
      desc: t('docs.integrations.claudeCodeDesc'),
      config: `export ANTHROPIC_BASE_URL="${base}"
export ANTHROPIC_AUTH_TOKEN="YOUR_API_KEY"
claude`,
    },
    {
      name: 'Cursor IDE',
      desc: t('docs.integrations.cursorDesc'),
      config: `Cursor Settings → Models → OpenAI API Key:
Override OpenAI Base URL: ON
Base URL: ${base}/v1
API Key:  YOUR_API_KEY`,
    },
    {
      name: 'OpenClaw Gateway',
      desc: t('docs.integrations.openclawDesc'),
      config: `upstreams:
  - id: "bestaiprice"
    protocol: "openai"
    base_url: "${base}/v1"
    api_key: "YOUR_API_KEY"`,
    },
    {
      name: 'n8n Automation',
      desc: t('docs.integrations.n8nDesc'),
      config: `OpenAI / Anthropic Node → Credentials:
Base URL: ${base}/v1
API Key:  YOUR_API_KEY`,
    },
    {
      name: 'Codex / Aider / Continue',
      desc: t('docs.integrations.codexDesc'),
      config: `export OPENAI_API_BASE="${base}/v1"
export OPENAI_API_KEY="YOUR_API_KEY"
aider --model openai/gpt-4o`,
    },
    {
      name: 'Google Gemini CLI & SDK',
      desc: t('docs.integrations.geminiDesc'),
      config: `export GEMINI_API_KEY="YOUR_API_KEY"
export GEMINI_BASE_URL="${base}"`,
    },
    {
      name: 'LangChain / Multi-Agent',
      desc: t('docs.integrations.langchainDesc'),
      config: `from langchain_openai import ChatOpenAI
llm = ChatOpenAI(base_url="${base}/v1", api_key="YOUR_API_KEY", model="gpt-4o")`,
    },
  ]
})

const faqItems = computed(() => [
  { q: t('support.faq_topup_q'), a: t('support.faq_topup_a') },
  { q: t('support.faq_key_q'), a: t('support.faq_key_a') },
  { q: t('support.faq_models_q'), a: t('support.faq_models_a') },
  { q: t('support.faq_errors_q'), a: t('support.faq_errors_a') },
  { q: t('support.faq_charges_q'), a: t('support.faq_charges_a') },
  { q: t('support.faq_failover_q'), a: t('support.faq_failover_a') },
  { q: t('support.faq_limits_q'), a: t('support.faq_limits_a') },
])

const disputeContextText = computed(() => {
  const user = authStore.user
  return [
    `user_id: ${user?.id ?? 'guest'}`,
    `request_id: ${disputeRequestId.value.trim() || 'n/a'}`,
    `model: ${disputeModelName.value.trim() || 'n/a'}`,
    `page: ${route.fullPath}`,
    `timestamp: ${new Date().toISOString()}`,
  ].join('\n')
})
</script>
