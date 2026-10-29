# Claude Code Integration — BestAIPrice

Run Anthropic's **Claude Code** CLI powered by the **BestAIPrice** pool hypervisor and smart routing gateway.

## Why use BestAIPrice with Claude Code?
- **Automatic 429 Failover & Rebinding**: If an upstream account hits rate limits or cooldowns, BestAIPrice's session hypervisor automatically rebinds your CLI session without breaking your context.
- **Cost Reduction**: Access Claude Sonnet 5 / Claude Opus 5 at high-volume pooled rates with full prompt cache support.
- **One Unified API Key**: Single key access across all models.

---

## 🚀 Quick Setup (1 Minute)

### Option 1: Environment Variables (Recommended)
Add the following to your `~/.bashrc`, `~/.zshrc`, or project `.env`:

```bash
# Point Claude Code to BestAIPrice
export ANTHROPIC_BASE_URL="https://bestaiprice.com/gateway"
export ANTHROPIC_AUTH_TOKEN="sk-bestai-xxxxxxxxxxxxxxxxxxxxxxxx"

# Optional: Default model selector
export ANTHROPIC_MODEL="claude-sonnet-5"
```

Verify by starting Claude Code:
```bash
claude
```

---

### Option 2: Using the helper script `env.sh`
```bash
source integrations/claude-code/env.sh
claude
```

---

## 🛠️ Claude Code Skill: BestAIPrice Router

You can register a specialized skill with Claude Code to query model pricing, switch models on the fly, and inspect active route health.

See [`claude-code-skill.md`](./claude-code-skill.md) for skill installation and usage commands.

---

## 🐛 Troubleshooting

| Issue | Cause | Fix |
|---|---|---|
| `401 Unauthorized` | Invalid or inactive key | Verify key in `https://bestaiprice.com/gateway/keys` |
| `402 Insufficient Balance` | Account balance is 0 | Top up balance via Billing -> Top-up |
| `429 Rate Limit` | Provider traffic spike | Hypervisor will auto-rebind within 1-2 retries |
| CLI prompts for login | Token not recognized | Ensure `ANTHROPIC_AUTH_TOKEN` is exported |
