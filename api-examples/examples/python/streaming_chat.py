"""BestAIPrice — Real-Time Streaming Example (Python)

Demonstrates token-by-token streaming with low latency and request ID inspection.
"""

import os
import sys
from openai import OpenAI

api_key = os.environ.get("BESTAIPRICE_API_KEY")
if not api_key:
    print("Error: BESTAIPRICE_API_KEY environment variable is required.", file=sys.stderr)
    sys.exit(1)

base_url = os.environ.get("BESTAIPRICE_BASE_URL", "https://api.bestaiprice.com").rstrip("/") + "/v1"

client = OpenAI(
    api_key=api_key,
    base_url=base_url,
)

model = os.environ.get("MODEL", "deepseek-chat")
print(f"Streaming from BestAIPrice using {model}...")

stream = client.chat.completions.create(
    model=model,
    messages=[{"role": "user", "content": "Write a 4-line poem about cloud gateways."}],
    stream=True,
)

print("\n--- Stream Output ---")
for chunk in stream:
    delta = chunk.choices[0].delta.content if chunk.choices and chunk.choices[0].delta else ""
    if delta:
        sys.stdout.write(delta)
        sys.stdout.flush()

print("\n\n--- Stream Completed ---")
