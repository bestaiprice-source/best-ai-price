# BestAIPrice — API 快速集成示例与 Agent 接入指南

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![OpenAI 兼容](https://img.shields.io/badge/API-OpenAI%20兼容-green.svg)](https://bestaiprice.com)
[![Anthropic 兼容](https://img.shields.io/badge/API-Anthropic%20兼容-orange.svg)](https://bestaiprice.com)
[![Gemini 兼容](https://img.shields.io/badge/API-Gemini%20兼容-purple.svg)](https://bestaiprice.com)

[English](README.md) | [中文说明](README_CN.md) | [Русский](README_RU.md)

BestAIPrice 官方开发者示例仓库。本仓库为开发者和 AI Agent 提供开箱即用的配置、代码示例与集成指南，帮助您快速接入统一的大模型路由网关。

---

## ⚡ 快速开始（三步接入）

### 1. 获取 API Key
1. 访问并在 [bestaiprice.com](https://bestaiprice.com) 注册登录。
2. 进入 **API 密钥 (API Keys)**，创建新密钥（格式为 `sk-bestai-...`）。
3. 配置系统环境变量：
   ```bash
   export BESTAIPRICE_API_KEY="sk-bestai-xxxxxxxxxxxxxxxxxxxx"
   ```
   > 🔒 **安全守则**：请勿将密钥提交至任何公开 Git 仓库。

### 2. 确定 Base URL
BestAIPrice 按照标准协议提供统一入口：
| 协议类型 | Base URL | 认证请求头 (Header) |
|---|---|---|
| **OpenAI 兼容** (Chat Completions, Embeddings) | `https://bestaiprice.com/gateway/v1` | `Authorization: Bearer $BESTAIPRICE_API_KEY` |
| **Anthropic 兼容** (Messages API, Claude Code) | `https://bestaiprice.com/gateway` | `x-api-key: $BESTAIPRICE_API_KEY` |
| **Google Gemini 兼容** (generateContent) | `https://bestaiprice.com/gateway` | `x-goog-api-key: $BESTAIPRICE_API_KEY` |

### 3. 发送首个测试请求

#### cURL (OpenAI 格式)
```bash
curl https://bestaiprice.com/gateway/v1/chat/completions \
  -H "Authorization: Bearer $BESTAIPRICE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "你好，BestAIPrice！"}]
  }'
```

#### cURL (Claude Messages 格式)
```bash
curl https://bestaiprice.com/gateway/v1/messages \
  -H "x-api-key: $BESTAIPRICE_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-5",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": "来自 Claude 的问候！"}]
  }'
```

---

## 🤖 核心模型对照表

| 厂商 | 模型 ID (Model ID) | 说明 |
|---|---|---|
| **Anthropic** | `claude-sonnet-5` | 均衡旗舰模型 |
| **Anthropic** | `claude-opus-5` | 最强推理能力 |
| **Anthropic** | `claude-haiku-5-5` | 极速轻量模型 |
| **OpenAI** | `gpt-4o` | 全能多模态旗舰 |
| **OpenAI** | `gpt-4o-mini` | 高性价比日常对话模型 |
| **OpenAI** | `o3-mini` / `o1` | 深度思维链推理模型 |
| **DeepSeek** | `deepseek-v4-flash` | DeepSeek V4 Flash 综合对话 |
| **DeepSeek** | `deepseek-v4-pro` | DeepSeek V4 Pro 强化推理 |
| **Google** | `gemini-3.1-pro` | 超长上下文旗舰模型 |
| **Google** | `gemini-2.5-flash` | 极速响应低延迟模型 |
| **阿里 / 通义** | `qwen3.5-plus` | 多语言全能模型 |
| **Moonshot / Kimi** | `kimi-k2.7-code` | 仓库级编程模型 |

---

## 📁 目录导航

- [cURL 脚本示例](./examples/curl/)
- [Python 示例 (OpenAI / Anthropic / LangChain)](./examples/python/)
- [Node.js 示例 (ESM / SDK / Native Fetch)](./examples/nodejs/)
- [Claude Code 深度集成](./integrations/claude-code/)
- [Cursor IDE 接入配置](./integrations/cursor/)
- [OpenClaw 网关代理](./integrations/openclaw/)
- [n8n 自动化工作流](./integrations/n8n/)
- [常见问题与支持 FAQ](./faq/FAQ.md)
