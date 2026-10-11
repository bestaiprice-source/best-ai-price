# Frequently Asked Questions (FAQ) & Support Guide

This guide covers all key operational aspects of BestAIPrice: billing, credentials, model compatibility, error resolution, account pool failover, and disputed charge claims.

---

## 1. 💳 Balance & Top-Up (Пополнение)

### Q: How do I top up my balance?
Go to **Billing** → **Top-Up** in your dashboard. We support:
- **Cryptocurrency (USDT TRC20/ERC20, BTC, ETH, TON, etc.)** via automated payment gateways.
- **Credit / Debit Cards & Fiat payment rails** where available.

### Q: How quickly is the balance credited?
Balances are credited automatically immediately upon transaction confirmation on the blockchain (typically 1–3 network confirmations, or instant for card payments).

---

## 2. 🔑 API Keys & Security (Ключи API)

### Q: Where do I generate my API key?
In the user portal under **API Keys** → **Create Key**. The full secret key (`sk-bestai-...`) is displayed **only once**. Save it securely in your password manager or environment configuration.

### Q: Can I restrict an API key to specific models?
Yes. When editing or creating a key, you can configure model whitelists, IP restrictions, and custom monthly spend limits.

### Q: What is the Zero-Secrets Security Policy?
- Client API keys are hashed in our database.
- Upstream provider credentials (such as pool tokens) are strictly isolated within the Hypervisor runtime.
- **Never share your secret key with support or third parties.**

---

## 3. 🤖 Models & Compatibility (Модели и совместимость)

### Q: How are models routed?
BestAIPrice acts as a smart gateway and session hypervisor. When you request a model like `claude-3-7-sonnet` or `gpt-4o`, the gateway automatically selects the lowest-latency, highest-capacity available node or upstream provider.

### Q: Do I need separate keys for Claude, OpenAI, and Gemini?
No. One single BestAIPrice API key works across all formats:
- OpenAI Chat Completions (`/v1/chat/completions`)
- Anthropic Messages API (`/v1/messages`)
- Google Gemini generateContent (`/v1beta/models/...`)
- OpenAI Responses API (`/responses`)

---

## 4. ⚠️ Error Codes & Troubleshooting (Ошибки)

| Status Code | Meaning | Cause | Action |
|---|---|---|---|
| **401 Unauthorized** | Invalid Token | Missing, mistyped, or expired API key. | Check `Authorization: Bearer <key>` or `x-api-key: <key>`. |
| **402 Payment Required** | Insufficient Balance | Account or key balance exhausted. | Top up balance in Billing section. |
| **429 Too Many Requests** | Rate Limit / Cooldown | High concurrency or upstream provider cooldown. | BestAIPrice auto-retries on backup routes. If repeated, check your key's RPM limit. |
| **502 / 503 Bad Gateway** | Upstream Error | Upstream AI vendor temporary downtime. | Automatic failover switches to secondary route within 250ms. |

---

## 5. 💰 Billing & Token Accounting (Списания и тарификация)

### Q: How is token usage charged?
Usage is billed strictly per token according to the published retail price per 1,000,000 tokens (or per generated image/video for media models).

### Q: Is Prompt Caching supported?
Yes. For models supporting prompt caching (e.g., Anthropic Claude, Gemini), cached input tokens receive substantial discounts (up to 90% cheaper).

### Q: How do I trace costs for each request?
Every API request writes an immutable usage entry containing:
- `Request ID` (`req_...`)
- Timestamp
- Model ID
- Prompt & completion token counts
- Billed cost in USD

---

## 6. 🔄 Automatic Failover & Session Hypervisor (Failover)

### Q: What is the Session Hypervisor?
For heavy workloads (such as Claude Code CLI sessions), BestAIPrice manages a dynamic pool of accounts.
- **Affinity**: Keeps requests sticky to healthy nodes to maximize local cache hits.
- **Rebinding**: If an upstream node receives a 429 or becomes unavailable, the Hypervisor automatically rebinds the session to a backup node without dropping the client stream.

---

## 7. ⚡ Limits & Quotas (Лимиты)

### Q: What rate limits apply?
Rate limits depend on your account tier and key settings:
- Requests Per Minute (RPM)
- Tokens Per Minute (TPM)
- Concurrency protection: Accounts with very low balances have concurrency constrained to prevent negative overdrafts.

---

## 8. 🎫 Disputed Charges & Support Context Policy (Спорные списания)

### Q: How do I dispute a charge?
If you believe a request was incorrectly billed or failed midway:
1. Open the **Usage Log** in your dashboard.
2. Copy the corresponding **Request ID** (`req_01j7...`).
3. Click the **Support Chat Widget** in the lower-right corner of the dashboard.
4. Paste the Request ID and copy the generated **Support Context**.

### 🔒 Support Context Policy
The support context automatically packages:
- `user_id`: Your user identifier
- `request_id`: The transaction identifier
- `model`: Model requested
- `timestamp`: UTC timestamp of the request

> ⚠️ **Strict Security Rule**: The support context **never contains upstream secret keys or client API keys**. With the `request_id`, the system administrator can safely inspect the server-side audit logs, upstream vendor transaction record, and issue an immediate refund or credit adjustment if warranted.
