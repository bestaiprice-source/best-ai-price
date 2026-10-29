#!/usr/bin/env bash
set -euo pipefail

API_KEY="${BESTAIPRICE_API_KEY:-}"
if [ -z "$API_KEY" ]; then
  echo "Error: BESTAIPRICE_API_KEY is not set." >&2
  echo "Run: export BESTAIPRICE_API_KEY=\"sk-bestai-xxxx\"" >&2
  exit 1
fi

BASE_URL="${BESTAIPRICE_BASE_URL:-https://bestaiprice.com/gateway}"
MODEL="${1:-claude-sonnet-5}"

echo "Sending Anthropic Messages request to ${BASE_URL}/v1/messages (model: ${MODEL})..."

curl -s -i -X POST "${BASE_URL}/v1/messages" \
  -H "x-api-key: ${API_KEY}" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"${MODEL}\",
    \"max_tokens\": 512,
    \"messages\": [
      {\"role\": \"user\", \"content\": \"Write a short haiku about reliable AI routing.\"}
    ]
  }"
