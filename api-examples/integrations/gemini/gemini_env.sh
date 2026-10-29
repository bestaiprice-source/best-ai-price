#!/usr/bin/env bash
# BestAIPrice Gemini environment setup

export GEMINI_API_KEY="${BESTAIPRICE_API_KEY:-}"
export GEMINI_BASE_URL="${BESTAIPRICE_BASE_URL:-https://bestaiprice.com/gateway}"

echo "✅ Gemini CLI / SDK configured:"
echo "   Base URL: ${GEMINI_BASE_URL}"
echo "   Endpoint: ${GEMINI_BASE_URL}/v1beta"
