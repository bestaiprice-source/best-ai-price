# Node.js Examples — BestAIPrice

Fast, modern ESM examples for Node.js (v18+) using official client libraries or zero-dependency native `fetch`.

## Quick Start

1. Install dependencies:
   ```bash
   npm install
   ```

2. Export your API key:
   ```bash
   export BESTAIPRICE_API_KEY="sk-bestai-xxxxxxxxxxxxxxxx"
   ```

3. Run any example:
   ```bash
   # Native fetch with streaming (zero external dependencies)
   node fetch_chat.mjs

   # Official OpenAI SDK
   node openai_sdk.mjs

   # Official Anthropic SDK
   node anthropic_sdk.mjs
   ```
