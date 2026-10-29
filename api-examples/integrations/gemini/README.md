# Google Gemini Integration — BestAIPrice

BestAIPrice natively supports Google Gemini's REST protocol (`/v1beta/models/...:generateContent` and `:streamGenerateContent`) as well as OpenAI-compatible routing for all Gemini models.

---

## 🚀 Setup & Usage

### 1. Environment Configuration
```bash
export GEMINI_API_KEY="$BESTAIPRICE_API_KEY"
export GEMINI_BASE_URL="https://bestaiprice.com/gateway"
```

### 2. Available Models
- `gemini-3.1-pro` (Long-context reasoning)
- `gemini-2.5-flash` (Fast, high throughput)
- `gemini-1.5-flash` (Legacy, still served)

---

## 📡 Protocol 1: Native Gemini REST API
```bash
curl -X POST "https://bestaiprice.com/gateway/v1beta/models/gemini-2.5-flash:generateContent" \
  -H "x-goog-api-key: $BESTAIPRICE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [
      {
        "parts": [{"text": "Explain quantum entanglement in 1 simple analogy."}]
      }
    ]
  }'
```

## 📡 Protocol 2: OpenAI Compatibility API
You can also use Gemini models seamlessly with any OpenAI client:
```python
from openai import OpenAI
import os

client = OpenAI(
    api_key=os.environ["BESTAIPRICE_API_KEY"],
    base_url="https://bestaiprice.com/gateway/v1",
)

resp = client.chat.completions.create(
    model="gemini-2.5-flash",
    messages=[{"role": "user", "content": "Hello Gemini via OpenAI SDK!"}],
)
print(resp.choices[0].message.content)
```
