"""BestAIPrice — Google Gemini Endpoint Example (Python)

Calls BestAIPrice Gemini v1beta endpoint using requests.
"""

import os
import sys
import requests

api_key = os.environ.get("BESTAIPRICE_API_KEY")
if not api_key:
    print("Error: BESTAIPRICE_API_KEY environment variable is required.", file=sys.stderr)
    sys.exit(1)

base_url = os.environ.get("BESTAIPRICE_BASE_URL", "https://bestaiprice.com/gateway").rstrip("/")
model = os.environ.get("MODEL", "gemini-2.5-flash")

url = f"{base_url}/v1beta/models/{model}:generateContent"

payload = {
    "contents": [
        {
            "role": "user",
            "parts": [{"text": "What makes Gemini 2.5 Flash ideal for agent workflows?"}]
        }
    ]
}

headers = {
    "x-goog-api-key": api_key,
    "Content-Type": "application/json"
}

print(f"Calling Gemini endpoint at {url}...")
resp = requests.post(url, json=payload, headers=headers)

if resp.status_code != 200:
    print(f"Error {resp.status_code}: {resp.text}", file=sys.stderr)
    sys.exit(1)

data = resp.json()
print("\n--- Response ---")
candidates = data.get("candidates", [])
if candidates:
    parts = candidates[0].get("content", {}).get("parts", [])
    for p in parts:
        print(p.get("text", ""))
else:
    print(data)
