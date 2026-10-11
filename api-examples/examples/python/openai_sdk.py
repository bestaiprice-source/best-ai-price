"""BestAIPrice — OpenAI SDK Integration Example

Requires:
    pip install openai
"""

import os
import sys
from openai import OpenAI, APIError

api_key = os.environ.get("BESTAIPRICE_API_KEY")
if not api_key:
    print("Error: BESTAIPRICE_API_KEY environment variable is required.", file=sys.stderr)
    sys.exit(1)

base_url = os.environ.get("BESTAIPRICE_BASE_URL", "https://api.bestaiprice.com").rstrip("/") + "/v1"

client = OpenAI(
    api_key=api_key,
    base_url=base_url,
)

model = os.environ.get("MODEL", "gpt-4o")

print(f"Connecting to BestAIPrice via OpenAI SDK at {base_url} with model {model}...")

try:
    response = client.chat.completions.create(
        model=model,
        messages=[
            {"role": "system", "content": "You are a concise engineering assistant."},
            {"role": "user", "content": "List 3 advantages of using an AI API relay."},
        ],
        temperature=0.7,
    )
    print("\n--- Response ---")
    print(response.choices[0].message.content)
    print("\nUsage Tokens:", response.usage)
except APIError as e:
    print(f"API Error ({e.status_code}): {e.message}", file=sys.stderr)
    sys.exit(1)
