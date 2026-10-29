# BestAIPrice — Примеры интеграции API и подключение AI-агентов

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![OpenAI Compatible](https://img.shields.io/badge/API-OpenAI%20Compatible-green.svg)](https://bestaiprice.com)
[![Anthropic Compatible](https://img.shields.io/badge/API-Anthropic%20Compatible-orange.svg)](https://bestaiprice.com)
[![Gemini Compatible](https://img.shields.io/badge/API-Gemini%20Compatible-purple.svg)](https://bestaiprice.com)

[English](README.md) | [中文说明](README_CN.md) | [Русский](README_RU.md)

Официальный репозиторий интеграций **BestAIPrice**. Здесь собраны проверенные примеры кода, конфигурационные файлы для CLI и готовые пресеты для быстрого подключения любого приложения, терминального агента или рабочего процесса к шлюзу BestAIPrice.

---

## ⚡ Быстрый старт (3 шага)

### 1. Получение API-ключа
1. Зарегистрируйтесь или войдите в личный кабинет на [bestaiprice.com](https://bestaiprice.com).
2. Перейдите в раздел **API Keys** и создайте новый ключ (`sk-bestai-...`).
3. Задайте переменную окружения:
   ```bash
   export BESTAIPRICE_API_KEY="sk-bestai-xxxxxxxxxxxxxxxxxxxx"
   ```
   > 🔒 **Безопасность**: Никогда не коммитьте секретный ключ в git-репозитории. Все примеры считывают ключ из переменной окружения `BESTAIPRICE_API_KEY`.

### 2. Выбор Base URL и заголовка авторизации
| Протокол / Инструмент | Base URL | Заголовок авторизации |
|---|---|---|
| **OpenAI Compatible** (Chat Completions, Embeddings, Codex) | `https://bestaiprice.com/gateway/v1` | `Authorization: Bearer $BESTAIPRICE_API_KEY` |
| **Anthropic Compatible** (Messages API, Claude Code) | `https://bestaiprice.com/gateway` | `x-api-key: $BESTAIPRICE_API_KEY` |
| **Google Gemini Compatible** (generateContent) | `https://bestaiprice.com/gateway` | `x-goog-api-key: $BESTAIPRICE_API_KEY` |

### 3. Первый запрос

#### cURL (OpenAI Chat Completions)
```bash
curl https://bestaiprice.com/gateway/v1/chat/completions \
  -H "Authorization: Bearer $BESTAIPRICE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Привет, BestAIPrice!"}]
  }'
```

#### cURL (Anthropic Claude Messages)
```bash
curl https://bestaiprice.com/gateway/v1/messages \
  -H "x-api-key: $BESTAIPRICE_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-5",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": "Привет от Claude!"}]
  }'
```

---

## 🤖 Модели из каталога

| Провайдер | Model ID | Назначение |
|---|---|---|
| **Anthropic** | `claude-sonnet-5` | Сбалансированный флагман |
| **Anthropic** | `claude-opus-5` | Максимальные рассуждения |
| **Anthropic** | `claude-haiku-5-5` | Быстрая и экономичная модель |
| **OpenAI** | `gpt-4o` | Мультимодальный флагман |
| **OpenAI** | `gpt-4o-mini` | Оптимальная для повседневных задач |
| **OpenAI** | `o3-mini` / `o1` | Глубокие рассуждения и логика (CoT) |
| **DeepSeek** | `deepseek-v4-flash` | DeepSeek V4 Flash для универсальных диалогов |
| **DeepSeek** | `deepseek-v4-pro` | DeepSeek V4 Pro — рассуждающая модель |
| **Google** | `gemini-3.1-pro` | Сверхдлинный контекст |
| **Google** | `gemini-2.5-flash` | Минимальная задержка и высокая скорость |
| **Qwen / Alibaba** | `qwen3.5-plus` | Многоязычная универсальная модель |
| **Moonshot / Kimi** | `kimi-k2.7-code` | Кодер для работы с целым репозиторием |

---

## 📁 Структура каталогов

```text
api-examples/
├── README.md                      # Документация на английском
├── README_CN.md                   # Документация на китайском
├── README_RU.md                   # Документация на русском
├── LICENSE                        # MIT License
├── examples/
│   ├── curl/                      # Готовые bash-скрипты для cURL
│   ├── python/                    # Python SDK (OpenAI, Anthropic, LangChain, Streaming)
│   └── nodejs/                    # Node.js ESM (OpenAI, Anthropic, нативный fetch)
├── integrations/
│   ├── claude-code/               # Настройка Claude Code CLI и сессионного failover
│   ├── codex/                     # Aider, Continue.dev, Copilot
│   ├── gemini/                    # Gemini CLI и Google GenAI SDK
│   ├── cursor/                    # Инструкции для Cursor IDE и готовый .cursorrules
│   ├── openclaw/                  # Готовый config.yaml для OpenClaw прокси
│   └── n8n/                       # Готовый импортируемый workflow для n8n
└── faq/
    └── FAQ.md                     # Подробный FAQ по балансу, ошибкам и спорным списаниям
```

---

## 🛡️ Политика Zero-Secrets и поддержка (§15)

- **Безопасность**: Никаких секретов в коде — всё передается через переменные окружения.
- **Трассировка запросов**: Каждый ответ возвращает заголовок `x-request-id`.
- **Спорные списания**: При вопросах по списанию отправьте администратору `request_id` из истории использования. По нему администратор восстанавливает точную запись биллинга и фактическую себестоимость в апстриме.
