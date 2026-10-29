/**
 * BestAIPrice — Official @anthropic-ai/sdk npm Example
 */

import Anthropic from '@anthropic-ai/sdk';

const apiKey = process.env.BESTAIPRICE_API_KEY;
if (!apiKey) {
  console.error('Error: BESTAIPRICE_API_KEY environment variable is required.');
  process.exit(1);
}

const baseURL = (process.env.BESTAIPRICE_BASE_URL || 'https://bestaiprice.com/gateway').replace(/\/+$/, '');
const model = process.env.MODEL || 'claude-sonnet-5';

const anthropic = new Anthropic({
  apiKey,
  baseURL,
});

console.log(`Connecting to BestAIPrice at ${baseURL} using official Anthropic SDK (${model})...`);

try {
  const message = await anthropic.messages.create({
    model,
    max_tokens: 350,
    messages: [
      { role: 'user', content: 'Explain prompt caching and token optimization in 2 sentences.' },
    ],
  });

  console.log('\n--- Claude Response ---');
  for (const block of message.content) {
    if (block.type === 'text') {
      console.log(block.text);
    }
  }
  console.log('\nUsage:', message.usage);
} catch (error) {
  console.error('Anthropic SDK error:', error.message);
  process.exit(1);
}
