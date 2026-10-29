#!/usr/bin/env bash
set -euo pipefail

API_KEY="${BESTAIPRICE_API_KEY:-}"
if [ -z "$API_KEY" ]; then
  echo "Error: BESTAIPRICE_API_KEY is not set." >&2
  echo "Run: export BESTAIPRICE_API_KEY=\"sk-bestai-xxxx\"" >&2
  exit 1
fi

BASE_URL="${BESTAIPRICE_BASE_URL:-https://bestaiprice.com/gateway}"
MODEL="${1:-gpt-4o}"

echo "Streaming OpenAI Chat Completion tokens from ${BASE_URL}/v1/chat/completions (model: ${MODEL})..."

curl -N --no-buffer -X POST "${BASE_URL}/v1/chat/completions" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d "{
    \"model\": \"${MODEL}\",
    \"stream\": true,
    \"messages\": [
      {\"role\": \"user\", \"content\": \"Count from 1 to 10 slowly with words.\"}
    ]
  }"
