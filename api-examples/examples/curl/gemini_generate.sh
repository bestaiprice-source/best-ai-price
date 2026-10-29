#!/usr/bin/env bash
set -euo pipefail

API_KEY="${BESTAIPRICE_API_KEY:-}"
if [ -z "$API_KEY" ]; then
  echo "Error: BESTAIPRICE_API_KEY is not set." >&2
  echo "Run: export BESTAIPRICE_API_KEY=\"sk-bestai-xxxx\"" >&2
  exit 1
fi

BASE_URL="${BESTAIPRICE_BASE_URL:-https://bestaiprice.com/gateway}"
MODEL="${1:-gemini-2.5-flash}"

echo "Sending Gemini generateContent to ${BASE_URL}/v1beta/models/${MODEL}:generateContent..."

curl -s -i -X POST "${BASE_URL}/v1beta/models/${MODEL}:generateContent" \
  -H "x-goog-api-key: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [
      {
        "role": "user",
        "parts": [{"text": "Summarize the benefit of pooled AI account failover in 2 bullet points."}]
      }
    ]
  }'
