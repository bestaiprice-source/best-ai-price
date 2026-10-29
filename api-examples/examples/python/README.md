# Python Examples — BestAIPrice

Ready-to-use Python examples for connecting to BestAIPrice with the official `openai`, `anthropic`, and `langchain` packages.

## Installation

Create a virtual environment and install dependencies:
```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

Export your API key:
```bash
export BESTAIPRICE_API_KEY="sk-bestai-xxxxxxxxxxxxxxxx"
```

## Available Scripts

1. **`openai_sdk.py`**
   Uses official `openai` SDK with `base_url="https://bestaiprice.com/gateway/v1"`.
   ```bash
   python openai_sdk.py
   ```

2. **`anthropic_sdk.py`**
   Uses official `anthropic` SDK with `base_url="https://bestaiprice.com/gateway"`.
   ```bash
   python anthropic_sdk.py
   ```

3. **`streaming_chat.py`**
   Streams tokens in real-time to stdout with request ID capture for debugging.
   ```bash
   python streaming_chat.py
   ```

4. **`gemini_sdk.py`**
   Invokes Google Gemini endpoints with zero SDK bloat.
   ```bash
   python gemini_sdk.py
   ```

5. **`langchain_agent.py`**
   Initializes `ChatOpenAI` from `langchain-openai` pointing to the BestAIPrice gateway.
   ```bash
   python langchain_agent.py
   ```
