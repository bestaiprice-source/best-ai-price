# BestAIPrice — API Examples & Agent Integrations

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![OpenAI Compatible](https://img.shields.io/badge/API-OpenAI%20Compatible-green.svg)](https://bestaiprice.com)
[![Anthropic Compatible](https://img.shields.io/badge/API-Anthropic%20Compatible-orange.svg)](https://bestaiprice.com)
[![Gemini Compatible](https://img.shields.io/badge/API-Gemini%20Compatible-purple.svg)](https://bestaiprice.com)
[![Status: Production Ready](https://img.shields.io/badge/Status-Verified-brightgreen.svg)](https://bestaiprice.com)

[English](README.md) | [中文说明](README_CN.md)

Welcome to the official developer integration repository for **BestAIPrice**.  
This repository provides production-tested code samples, CLI configurations, and agent setups for connecting any application, coding agent, or automation workflow to the BestAIPrice API Gateway.

---

## ⚡ Quick Start (3 Steps)

### 1. Get Your API Key
1. Register or log in at [bestaiprice.com](https://bestaiprice.com).
2. Go to **API Keys** and generate a new key (`sk-bestai-...`).
3. Set your environment variable:
   ```bash
   export BESTAIPRICE_API_KEY="sk-bestai-xxxxxxxxxxxxxxxxxxxx"
   ```
   > 🔒 **Security Notice**: Never hardcode your API key into git repositories. All examples here read from `BESTAIPRICE_API_KEY`.

### 2. Configure Your Base URL
BestAIPrice provides unified protocol endpoints:
| Protocol / Service | Base URL | Auth Header |
|---|---|---|
| **OpenAI Compatible** (Chat, Embeddings, Codex) | `https://api.bestaiprice.com/v1` | `Authorization: Bearer $BESTAIPRICE_API_KEY` |
| **Anthropic Compatible** (Messages API, Claude Code) | `https://api.bestaiprice.com` | `x-api-key: $BESTAIPRICE_API_KEY` |
| **Google Gemini Compatible** (generateContent) | `https://api.bestaiprice.com` | `x-goog-api-key: $BESTAIPRICE_API_KEY` |

> *Self-hosted gateway users:* Replace `https://api.bestaiprice.com` with your self-hosted domain (e.g., `https://your-domain.com/gateway`).

### 3. Send Your First Request

#### cURL (OpenAI Chat Completions)
```bash
curl https://api.bestaiprice.com/v1/chat/completions \
  -H "Authorization: Bearer $BESTAIPRICE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello BestAIPrice!"}]
  }'
```

#### cURL (Anthropic Claude Messages)
```bash
curl https://api.bestaiprice.com/v1/messages \
  -H "x-api-key: $BESTAIPRICE_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-7-sonnet",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": "Hello from Claude!"}]
  }'
```

---

## 🤖 Supported Models (Synchronized with Catalog)

BestAIPrice routes across best-price upstream providers with zero vendor lock-in. Active model IDs include:

| Provider | Canonical Model ID | Description |
|---|---|---|
| **Anthropic** | `claude-3-7-sonnet` | Claude 3.7 Sonnet (Hybrid reasoning) |
| **Anthropic** | `claude-3-5-sonnet` | Claude 3.5 Sonnet (Coding champion) |
| **Anthropic** | `claude-3-5-haiku` | Claude 3.5 Haiku (Ultra-fast & economical) |
| **OpenAI** | `gpt-4o` | GPT-4o Flagship Multimodal |
| **OpenAI** | `gpt-4o-mini` | Fast & lightweight model |
| **OpenAI** | `o3-mini` / `o1` | High-tier reasoning models |
| **DeepSeek** | `deepseek-chat` | DeepSeek-V3 General Chat |
| **DeepSeek** | `deepseek-reasoner` | DeepSeek-R1 Chain-of-Thought Reasoning |
| **Google** | `gemini-2.5-pro` | Gemini 2.5 Pro (Massive context) |
| **Google** | `gemini-2.5-flash` | Gemini 2.5 Flash (Low latency) |
| **Qwen / Alibaba** | `qwen-2.5-coder-32b` | Open-source code powerhouse |

To query the live list of models and pricing programmatically:
```bash
curl https://api.bestaiprice.com/v1/models \
  -H "Authorization: Bearer $BESTAIPRICE_API_KEY"
```

---

## 📁 Repository Structure

```text
api-examples/
├── README.md                      # Main guide (English)
├── README_CN.md                   # Main guide (Chinese)
├── LICENSE                        # MIT License
├── examples/
│   ├── curl/                      # Direct shell scripts for all protocols
│   ├── python/                    # Python SDKs (OpenAI, Anthropic, LangChain, Streaming)
│   └── nodejs/                    # Node.js ESM (OpenAI, Anthropic, native fetch)
├── integrations/
│   ├── claude-code/               # Claude Code CLI setup & routing
│   ├── codex/                     # Codex / Copilot / Continue / Aider
│   ├── gemini/                    # Gemini SDK & CLI setup
│   ├── cursor/                    # Cursor IDE .cursorrules & configuration
│   ├── openclaw/                  # OpenClaw proxy & YAML config
│   └── n8n/                       # n8n workflows & custom credentials
└── faq/
    └── FAQ.md                     # Complete billing, error code & support FAQ
```

---

## 🛠️ Integrations Overview

- **[Claude Code CLI](./integrations/claude-code/)**: Route Claude Code commands through BestAIPrice Claude Max capacity pool with automatic failover.
- **[Cursor IDE](./integrations/cursor/)**: Configure custom OpenAI endpoint in Cursor and enforce project coding standards with `.cursorrules`.
- **[OpenClaw AI Gateway](./integrations/openclaw/)**: Setup high-performance upstream proxy with rate-limiting and route fallbacks.
- **[n8n Workflow Automation](./integrations/n8n/)**: Plug BestAIPrice into n8n AI nodes with pre-built workflow templates.
- **[Codex / Continue / Aider](./integrations/codex/)**: Terminal-based coding agents pointed directly at BestAIPrice.
- **[Gemini Ecosystem](./integrations/gemini/)**: Use standard Google GenAI and Gemini client libraries.

---

## 🛡️ Security & Zero-Secrets Policy

1. **No Secrets in Code**: All examples in this repository parse environment variables.
2. **Support Tracing**: Every gateway request returns an `x-request-id` header (e.g., `req_01j7...`). In support tickets, provide this `x-request-id`. **Never share your secret API key with support agents.**
3. **Upstream Isolation**: Upstream provider keys are managed internally by the Hypervisor and are never leaked to client responses.

---

## 🤝 Support & Disputed Charges

- **Documentation & FAQ**: [FAQ.md](./faq/FAQ.md)
- **Support Portal**: Available on the dashboard widget at [bestaiprice.com](https://bestaiprice.com)
- **Disputed Charge Policy**: If you notice an unexpected balance deduction, locate the corresponding `Request ID` in your dashboard **Usage Log** and submit it to the support widget. Admins can look up the exact upstream transaction log and billing item.
