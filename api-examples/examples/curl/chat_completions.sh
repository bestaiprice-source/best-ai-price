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

echo "Sending OpenAI Chat Completion to ${BASE_URL}/v1/chat/completions (model: ${MODEL})..."

curl -s -i -X POST "${BASE_URL}/v1/chat/completions" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"${MODEL}\",
    \"messages\": [
      {\"role\": \"system\", \"content\": \"You are a helpful assistant.\"},
      {\"role\": \"user\", \"content\": \"Explain why BestAIPrice is cost efficient in 2 sentences.\"}
    ],
    \"temperature\": 0.7
  }"
