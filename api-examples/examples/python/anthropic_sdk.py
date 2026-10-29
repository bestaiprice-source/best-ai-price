"""BestAIPrice — Anthropic SDK Integration Example

Requires:
    pip install anthropic
"""

import os
import sys
from anthropic import Anthropic, APIError

api_key = os.environ.get("BESTAIPRICE_API_KEY")
if not api_key:
    print("Error: BESTAIPRICE_API_KEY environment variable is required.", file=sys.stderr)
    sys.exit(1)

base_url = os.environ.get("BESTAIPRICE_BASE_URL", "https://bestaiprice.com/gateway").rstrip("/")

client = Anthropic(
    api_key=api_key,
    base_url=base_url,
)

model = os.environ.get("MODEL", "claude-sonnet-5")

print(f"Connecting to BestAIPrice via Anthropic SDK at {base_url} with model {model}...")

try:
    message = client.messages.create(
        model=model,
        max_tokens=300,
        messages=[
            {"role": "user", "content": "Explain in one sentence why developers love Anthropic Claude models."}
        ],
    )
    print("\n--- Response ---")
    for block in message.content:
        if getattr(block, "text", None):
            print(block.text)
    print(f"\nModel: {message.model} | Input Tokens: {message.usage.input_tokens} | Output Tokens: {message.usage.output_tokens}")
except APIError as e:
    print(f"Anthropic API Error ({e.status_code}): {e.message}", file=sys.stderr)
    sys.exit(1)
