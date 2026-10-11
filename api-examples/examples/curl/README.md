# cURL Examples — BestAIPrice

This folder contains tested shell scripts for querying BestAIPrice using standard `curl`.

## Prerequisites
Ensure `BESTAIPRICE_API_KEY` is exported:
```bash
export BESTAIPRICE_API_KEY="sk-bestai-xxxxxxxxxxxxxxxx"
# Optional custom base url (default: https://api.bestaiprice.com)
export BESTAIPRICE_BASE_URL="https://api.bestaiprice.com"
```

## Scripts
- **`chat_completions.sh`**: Standard OpenAI `/v1/chat/completions` request.
- **`anthropic_messages.sh`**: Anthropic `/v1/messages` request.
- **`gemini_generate.sh`**: Gemini `/v1beta/models/...:generateContent` request.
- **`streaming_sse.sh`**: Real-time Server-Sent Events (SSE) token stream with unbuffered curl.
- **`list_models.sh`**: Query `/v1/models` catalog.

Run any script directly:
```bash
chmod +x *.sh
./chat_completions.sh
./streaming_sse.sh
```
