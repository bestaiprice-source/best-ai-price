# OpenClaw Integration — BestAIPrice

Configure [OpenClaw](https://github.com/) / AI Gateway proxy to connect to BestAIPrice as an upstream provider with load balancing, caching, and multi-tenant billing.

---

## 🛠️ Configuration

OpenClaw supports OpenAI-compatible and Anthropic-compatible upstreams.

Use the provided [`config.yaml`](./config.yaml):

```yaml
version: "1.0"

upstreams:
  - id: "bestaiprice-openai"
    name: "BestAIPrice Gateway (OpenAI)"
    protocol: "openai"
    base_url: "https://bestaiprice.com/gateway/v1"
    api_key: "${BESTAIPRICE_API_KEY}"
    models:
      - "gpt-4o"
      - "gpt-4o-mini"
      - "deepseek-v4-flash"
      - "deepseek-v4-pro"
      - "kimi-k2.7-code"

  - id: "bestaiprice-anthropic"
    name: "BestAIPrice Gateway (Claude)"
    protocol: "anthropic"
    base_url: "https://bestaiprice.com/gateway"
    api_key: "${BESTAIPRICE_API_KEY}"
    models:
      - "claude-sonnet-5"
      - "claude-opus-5"
      - "claude-haiku-5-5"

routes:
  - path: "/v1/chat/completions"
    upstream: "bestaiprice-openai"
  - path: "/v1/messages"
    upstream: "bestaiprice-anthropic"
```

## Running OpenClaw
```bash
openclaw --config config.yaml
```
OpenClaw will now bind to your local port and transparently route requests to BestAIPrice.
