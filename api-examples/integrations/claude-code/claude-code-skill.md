# BestAIPrice Claude Code Skill

Integrate BestAIPrice model lookup and routing diagnostics directly into your Claude Code session.

## Skill Registration
In Claude Code, you can register skills by saving custom prompt workflows into your project's `.claude/skills/` or prompt instructions.

### Capabilities
1. **Catalog Query**: Checks `/v1/models` to discover active models and current retail price per 1M tokens.
2. **Session Health Check**: Queries gateway health and reports roundtrip latency.
3. **Dispute Reporter**: Formats sanitized context with User ID and Request ID for support review.

### Sample Bash Command for Claude Code
```bash
# Check current BestAIPrice models from terminal inside Claude Code:
curl -s "${ANTHROPIC_BASE_URL}/v1/models" -H "Authorization: Bearer ${ANTHROPIC_AUTH_TOKEN}" | jq '.data[] | {id: .id, price: .pricing}'
```
