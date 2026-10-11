/**
 * BestAIPrice — Official OpenAI npm SDK Example
 */

import OpenAI from 'openai';

const apiKey = process.env.BESTAIPRICE_API_KEY;
if (!apiKey) {
  console.error('Error: BESTAIPRICE_API_KEY environment variable is required.');
  process.exit(1);
}

const rawBase = (process.env.BESTAIPRICE_BASE_URL || 'https://api.bestaiprice.com').replace(/\/+$/, '');
const baseURL = `${rawBase}/v1`;
const model = process.env.MODEL || 'gpt-4o';

const client = new OpenAI({
  apiKey,
  baseURL,
});

console.log(`Connecting to BestAIPrice at ${baseURL} using official OpenAI SDK (${model})...`);

try {
  const completion = await client.chat.completions.create({
    model,
    messages: [
      { role: 'user', content: 'What are the main benefits of model routing gateways?' },
    ],
  });

  console.log('\n--- Completion Result ---');
  console.log(completion.choices[0]?.message?.content);
  console.log('\nUsage:', completion.usage);
} catch (error) {
  console.error('OpenAI SDK error:', error.message);
  process.exit(1);
}
