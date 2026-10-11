/**
 * BestAIPrice — Native Fetch Example (Node.js 18+)
 * Zero external dependencies.
 */

const apiKey = process.env.BESTAIPRICE_API_KEY;
if (!apiKey) {
  console.error('Error: BESTAIPRICE_API_KEY environment variable is required.');
  process.exit(1);
}

const rawBase = (process.env.BESTAIPRICE_BASE_URL || 'https://api.bestaiprice.com').replace(/\/+$/, '');
const endpoint = `${rawBase}/v1/chat/completions`;
const model = process.env.MODEL || 'gpt-4o-mini';

console.log(`Sending native fetch request to ${endpoint} (${model})...`);

const response = await fetch(endpoint, {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${apiKey}`,
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({
    model,
    messages: [
      { role: 'system', content: 'You are a concise AI assistant.' },
      { role: 'user', content: 'Say hello and give one quick coding tip.' },
    ],
    temperature: 0.7,
  }),
});

const requestId = response.headers.get('x-request-id') || response.headers.get('request-id') || 'n/a';
console.log(`HTTP Status: ${response.status} | Request ID: ${requestId}`);

if (!response.ok) {
  const errText = await response.text();
  console.error(`Request failed: ${errText}`);
  process.exit(1);
}

const data = await response.json();
console.log('\n--- Model Response ---');
console.log(data.choices?.[0]?.message?.content);
if (data.usage) {
  console.log('\nToken Usage:', data.usage);
}
