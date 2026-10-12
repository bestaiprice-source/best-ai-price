# BestAIPrice Gateway & Session Hypervisor

<div align="center">

[![Go](https://img.shields.io/badge/Go-1.25.7-00ADD8.svg?style=flat&logo=go)](https://golang.org/)
[![Vue](https://img.shields.io/badge/Vue-3.4+-4FC08D.svg?style=flat&logo=vuedotjs)](https://vuejs.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-15+-336791.svg?style=flat&logo=postgresql)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-7+-DC382D.svg?style=flat&logo=redis)](https://redis.io/)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED.svg?style=flat&logo=docker)](https://www.docker.com/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Enterprise-Grade AI Gateway, Multi-Account Session Hypervisor & Cost Optimizer**

[Features](#-key-features) • [Architecture](#-architecture) • [Quick Start](#-quick-start) • [API & SDK Examples](#-api-and-integrations) • [Configuration](#-configuration) • [Documentation](#-documentation)

</div>

---

## 🚀 Overview

**BestAIPrice Gateway** is a high-performance, fault-tolerant AI Gateway and **AI Session Hypervisor (ASH / Yunwu Engine)**. It coordinates pools of upstream AI accounts (Claude Max 20x, OpenAI / ChatGPT Pro, Codex, Google Gemini, Moonshot / Kimi, DeepSeek, and third-party aggregators), distributing high-throughput capacity to end users through a single, unified API key.

The platform decouples client software (Claude Code CLI, Cursor, Continue, Codex, OpenAI / Anthropic SDKs) from physical accounts and upstream resellers, guaranteeing continuous service delivery even through upstream `429 Too Many Requests`, token exhaustion, network drops, or price changes.

```text
+-----------------------------------------------------------------------------------+
|                                  Client Requests                                  |
|         (Claude Code CLI, Cursor, OpenAI/Anthropic SDKs, Gemini CLI, n8n)         |
+-----------------------------------------------------------------------------------+
                                          |
                                          v
+-----------------------------------------------------------------------------------+
|                   BestAIPrice High-Throughput Gateway Layer                       |
|           • Unified Auth (Single Key for All Models)  • GNet / HTTP Ingress       |
|           • Pre-Admission Pricing Guard               • Anti-Overdraft Balance    |
+-----------------------------------------------------------------------------------+
                                          |
                                          v
+-----------------------------------------------------------------------------------+
|                      AI Session Hypervisor (ASH / Yunwu)                          |
|  • VirtualSession State Engine      • Context Compiler & Rebinding                |
|  • Dynamic Price-Based Routing      • Health Prober & Cooldown Controller         |
|  • Organic Token Load Balancer      • Cheaper Provider Auto-Scanner               |
+-----------------------------------------------------------------------------------+
            |                           |                           |
            v                           v                           v
+-----------------------+   +-----------------------+   +-----------------------+
|  Claude (Anthropic)   |   |   OpenAI & Codex      |   |  Multi-Provider Hub   |
|  • OAuth & Setup-Token|   |  • Responses API      |   |  • Portdan, OpenLux   |
|  • System Banner Inject|  |  • Native Chat Format |   |  • Top-API, Yuanqiao  |
|  • Context Compilation|   |  • Model Routing      |   |  • X-LLM, Custom Hub  |
+-----------------------+   +-----------------------+   +-----------------------+
```

---

## ✨ Key Features

### 1. Unified Gateway & Single API Key Access
- **Universal Single Key**: Issue a single API key to users that operates seamlessly across all permitted models—no need for separate keys per provider or model family.
- **Protocol Parity**:
  - Anthropic Messages API (`/v1/messages` with streaming SSE)
  - OpenAI Chat Completions (`/v1/chat/completions`) & Codex Responses API (`/responses`, `/v1/responses`)
  - Google Gemini Native API (`/v1beta/models/...`) & Gemini Live Bidirectional WebSocket
  - Multimodal media generation (Images, Audio, Video, and Sora endpoints)
  - Public model catalog and live pricing endpoint (`/v1/models`, `/models`)

### 2. AI Session Hypervisor (ASH)
- **Virtual Sessions & Session Affinity**: Locks conversational state to underlying physical nodes to preserve upstream prompt caching benefits.
- **Zero-Downtime Automatic Rebinding**: Upon encountering upstream `429 Too Many Requests`, timeouts, or node errors, ASH automatically acquires an alternative healthy execution node, recompiles prompt context (system instructions, summaries, recent messages), and transparently completes the request without client-visible failures.
- **Upstream Failover Loss Accounting**: If an upstream attempt fails mid-stream after burning tokens, the client is **never double-charged**; internal losses are attributed directly to operational margin metrics.

### 3. Intelligent Multi-Provider Hub & Price Routing
- **Primary / Backup Routing**: Automatically derives routes based on real-time price tiers—the cheapest provider serves as Primary, while the next-cheapest acts as Backup.
- **Continuous Cheaper Provider Checker**: Periodically inspects candidate providers against configured routes, groups opportunities by model family, calculates family-wide average savings, and alerts operators when cheaper providers are available.
- **Supported Aggregators**: Out-of-the-box support for OpenLux, Portdan, Top-API, Yuanqiaoyun, X-LLM, and arbitrary custom upstream endpoints.
- **Provider Balance Auto-Sync**: Queries upstream credit grants, token quotas, and wallet balances every few minutes with usage-based fallback estimation.

### 4. Real-Time Billing & Anti-Overdraft Protection
- **Atomic Compare-And-Swap (CAS)**: High-speed Redis balance cache with monotonic version tracking prevents concurrency race conditions and stale balance overwrites.
- **Strict Anti-Overdraft Guard**: Automatically clamps concurrency for low-balance pay-as-you-go users, and denies requests upfront if balance falls below the minimum operational threshold (`$0.002`).
- **Separate Money Ledger & API Logs**: Detailed ledger for deposits, usage deductions, refunds, and administrator adjustments, completely decoupled from technical request logs.

### 5. Administrative Observability & Telegram Alerts
- **Telegram Alert Bot**: Real-time notifications for 8 critical events (Provider Down, Upstream Key Invalid, Low Balance, Both Providers Down, Spike in 429/5xx, Failover Rate Exceeded, Cheaper Provider Found, Significant Price Change).
- **Deduplication & Distributed Cooldowns**: SHA-256 body fingerprinting and Redis distributed locks eliminate notification spam.
- **Gross Margin Analytics**: Real-time monitoring of Gross Revenue, Upstream Cost, Net Profit, and Gross Margin % with per-model and per-provider breakdowns.
- **Durable Audit Trail**: Append-only audit logs for all administrative modifications (provider changes, routing rules, balance adjustments).

---

## 🛠 Tech Stack

| Layer | Technologies |
|---|---|
| **Backend** | Go 1.25+ • Gin • GNet (High-Performance Event-Driven Ingress) • Ent ORM |
| **Frontend** | Vue 3 • Vite • TailwindCSS • Pinia • Vue Router • Lucide Icons |
| **Storage & Cache** | PostgreSQL 15+ • Redis 7+ (CAS Lua scripts & Distributed Locks) |
| **TLS & Networking** | Rust FFI / Sidecar • TLS Fingerprint Spoofing (`curl-cffi`, uTLS) |
| **Deployment** | Docker & Docker Compose • Systemd Service • Pre-built Binaries |

---

## 📦 Quick Start

### Option 1: Docker Compose (Recommended)

1. **Clone the repository**:
   ```bash
   git clone git@github.com:bestaiprice-source/best-ai-price.git
   cd best-ai-price/deploy
   ```

2. **Configure environment**:
   ```bash
   cp .env.example .env
   ```
   Generate secure secrets and update `.env`:
   ```bash
   # Generate secrets:
   openssl rand -hex 32 # Set as JWT_SECRET
   openssl rand -hex 32 # Set as TOTP_ENCRYPTION_KEY
   openssl rand -hex 32 # Set as POSTGRES_PASSWORD
   ```

3. **Start the stack**:
   ```bash
   docker compose -f docker-compose.local.yml up -d
   ```

4. **Access the Web Console**:
   Open `http://localhost:8080` in your browser. Complete the initial setup wizard or sign in with the configured credentials.

---

### Option 2: Local Development Setup

#### Prerequisites
- Go 1.25+
- Node.js 20+ & pnpm
- PostgreSQL 15+
- Redis 7+

#### Backend Setup
```bash
cd backend
cp .env.example .env # Set DB and Redis credentials
go mod download
go run cmd/server/main.go
```

#### Frontend Setup
```bash
cd frontend
pnpm install
pnpm dev
```

---

## 💡 API and Integrations

Clients communicate with the gateway using standard OpenAI or Anthropic protocols. Set the base URL to your BestAIPrice instance and pass your generated user API key.

### 1. Anthropic Messages Protocol
```bash
curl -X POST https://api.bestaiprice.com/v1/messages \
  -H "Authorization: Bearer <YOUR_API_KEY>" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-3-7-sonnet",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Explain quantum computing in simple terms."}
    ]
  }'
```

### 2. OpenAI Chat Completions Protocol
```bash
curl -X POST https://api.bestaiprice.com/v1/chat/completions \
  -H "Authorization: Bearer <YOUR_API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [
      {"role": "user", "content": "Write a hello world program in Go."}
    ]
  }'
```

### 3. Claude Code CLI Integration
```bash
export ANTHROPIC_BASE_URL="https://api.bestaiprice.com"
export ANTHROPIC_AUTH_TOKEN="<YOUR_API_KEY>"
claude
```

### 4. Codex & Cursor Integration
Configure your `.cursorrules` or OpenAI client settings:
```json
{
  "openai.baseUrl": "https://api.bestaiprice.com/v1",
  "openai.apiKey": "<YOUR_API_KEY>"
}
```

> 📖 **Comprehensive SDK & tool examples**: Explore the [`api-examples`](api-examples/) directory for ready-to-run code snippets in Python, Node.js, cURL, LangChain, n8n, OpenClaw, Continue, and Aider.

---

## ⚙️ Configuration Reference

Key environment variables to configure in `.env`:

| Variable | Default | Description |
|---|---|---|
| `SERVER_PORT` | `8080` | Gateway HTTP server listening port |
| `DATABASE_URL` | - | PostgreSQL connection string (`postgres://user:pass@host:5432/db`) |
| `REDIS_URL` | - | Redis connection string (`redis://:pass@host:6379/0`) |
| `JWT_SECRET` | - | 32+ byte secret for user authentication tokens |
| `TOTP_ENCRYPTION_KEY` | - | 32-byte AES key for encrypting provider credentials at rest |
| `ASH_ENABLED` | `true` | Enable AI Session Hypervisor core engine |
| `ASH_ENABLE_REBINDING` | `true` | Enable automatic failover & node rebinding on errors |
| `ASH_ENABLE_AFFINITY` | `true` | Enable virtual session affinity for prompt cache hits |
| `ASH_CHEAPER_PROVIDER_CHECK_SECONDS` | `21600` | Interval for cheaper provider background scans (6h) |
| `ASH_TELEGRAM_BOT_TOKEN` | - | Telegram Bot token for administrative alerts |
| `ASH_TELEGRAM_CHAT_ID` | - | Target Telegram channel or group ID for incident broadcasts |
| `ASH_TELEGRAM_ALLOWED_USERS` | - | Whitelist of Telegram user IDs/usernames allowed to manage the bot |

---

## 🧪 Testing

To run the complete unit test suite across all modules:

```bash
cd backend
go test ./...
```

For mock hypervisor execution without live upstream accounts, set `ASH_ENABLE_MOCK_RUNTIME=true` in your `.env`.

---

## 📚 Documentation

- [CLAUDE.md](CLAUDE.md) — Comprehensive technical master guide for developers and AI agents.
- [api-examples/](api-examples/) — Ready-to-use examples for Python, Node.js, cURL, and agent frameworks.
- [AUTH_API_GUIDE.md](AUTH_API_GUIDE.md) — Authentication, SSO, and user session specification.
- [COMMENTS_API_GUIDE.md](COMMENTS_API_GUIDE.md) — Provider review, comments, and community rating APIs.
- [DEV_GUIDE.md](DEV_GUIDE.md) — Local developer environment and contribution guidelines.

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).
