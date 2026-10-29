#!/usr/bin/env bash
# BestAIPrice Claude Code environment setup

if [ -z "${BESTAIPRICE_API_KEY:-}" ]; then
  echo "⚠️ Warning: BESTAIPRICE_API_KEY is not set."
  echo "Please export BESTAIPRICE_API_KEY=\"sk-bestai-xxxx\" before sourcing."
fi

export ANTHROPIC_BASE_URL="${BESTAIPRICE_BASE_URL:-https://bestaiprice.com/gateway}"
export ANTHROPIC_AUTH_TOKEN="${BESTAIPRICE_API_KEY:-}"
export ANTHROPIC_MODEL="${ANTHROPIC_MODEL:-claude-sonnet-5}"

echo "✅ Claude Code configured for BestAIPrice Gateway:"
echo "   Endpoint: ${ANTHROPIC_BASE_URL}"
echo "   Model:    ${ANTHROPIC_MODEL}"
echo "   Token:    ${ANTHROPIC_AUTH_TOKEN:0:10}..."
