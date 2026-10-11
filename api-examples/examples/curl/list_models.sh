#!/usr/bin/env bash
set -euo pipefail

API_KEY="${BESTAIPRICE_API_KEY:-}"
if [ -z "$API_KEY" ]; then
  echo "Error: BESTAIPRICE_API_KEY is not set." >&2
  echo "Run: export BESTAIPRICE_API_KEY=\"sk-bestai-xxxx\"" >&2
  exit 1
fi

BASE_URL="${BESTAIPRICE_BASE_URL:-https://api.bestaiprice.com}"

echo "Fetching active model catalog from ${BASE_URL}/v1/models..."

curl -s -X GET "${BASE_URL}/v1/models" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" | python3 -m json.tool || true
