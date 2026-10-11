# Codex & Coding Agent Integrations — BestAIPrice

Connect terminal coding tools, extensions, and autonomous agents (Aider, Continue.dev, Copilot CLI, Cline) to BestAIPrice via the standard OpenAI-compatible `/v1` endpoint.

---

## 1. Aider (Terminal Pair Programmer)

[Aider](https://aider.chat/) is an open-source command-line AI pair programmer.

### Quick Start with Environment Variables
```bash
export OPENAI_API_BASE="https://api.bestaiprice.com/v1"
export OPENAI_API_KEY="$BESTAIPRICE_API_KEY"

# Launch with your desired model:
aider --model openai/gpt-4o
# Or Claude via Anthropic format:
aider --model openai/claude-3-7-sonnet
```

### Or using `.aider.conf.yml`
Place [`aider_config.yml`](./aider_config.yml) as `.aider.conf.yml` in your repository root:
```yaml
openai-api-base: https://api.bestaiprice.com/v1
model: openai/gpt-4o
edit-format: diff
```

---

## 2. Continue.dev (VS Code & JetBrains Extension)

Add BestAIPrice to your `~/.continue/config.json`:
```json
{
  "models": [
    {
      "title": "BestAIPrice GPT-4o",
      "provider": "openai",
      "model": "gpt-4o",
      "apiBase": "https://api.bestaiprice.com/v1",
      "apiKey": "${BESTAIPRICE_API_KEY}"
    },
    {
      "title": "BestAIPrice Claude 3.7 Sonnet",
      "provider": "openai",
      "model": "claude-3-7-sonnet",
      "apiBase": "https://api.bestaiprice.com/v1",
      "apiKey": "${BESTAIPRICE_API_KEY}"
    },
    {
      "title": "BestAIPrice DeepSeek R1",
      "provider": "openai",
      "model": "deepseek-reasoner",
      "apiBase": "https://api.bestaiprice.com/v1",
      "apiKey": "${BESTAIPRICE_API_KEY}"
    }
  ]
}
```

---

## 3. OpenAI Responses API (Codex format)
BestAIPrice supports the `/responses` endpoint for agents requiring the new Codex responses protocol:
```bash
curl https://api.bestaiprice.com/responses \
  -H "Authorization: Bearer $BESTAIPRICE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "input": "Write a binary search algorithm in TypeScript"
  }'
```
