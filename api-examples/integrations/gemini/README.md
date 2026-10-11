# Google Gemini Integration — BestAIPrice

BestAIPrice natively supports Google Gemini's REST protocol (`/v1beta/models/...:generateContent` and `:streamGenerateContent`) as well as OpenAI-compatible routing for all Gemini models.

---

## 🚀 Setup & Usage

### 1. Environment Configuration
```bash
export GEMINI_API_KEY="$BESTAIPRICE_API_KEY"
export GEMINI_BASE_URL="https://api.bestaiprice.com"
```

### 2. Available Models
- `gemini-2.5-pro` (Long context reasoning)
- `gemini-2.5-flash` (Fast, high throughput)
- `gemini-1.5-pro`
- `gemini-1.5-flash`

---

## 📡 Protocol 1: Native Gemini REST API
```bash
curl -X POST "https://api.bestaiprice.com/v1beta/models/gemini-2.5-flash:generateContent" \
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
    base_url="https://api.bestaiprice.com/v1",
)

resp = client.chat.completions.create(
    model="gemini-2.5-flash",
    messages=[{"role": "user", "content": "Hello Gemini via OpenAI SDK!"}],
)
print(resp.choices[0].message.content)
```
