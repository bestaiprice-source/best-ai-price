"""BestAIPrice — LangChain Integration Example

Connects LangChain's ChatOpenAI directly to BestAIPrice.
"""

import os
import sys
from langchain_openai import ChatOpenAI
from langchain_core.messages import HumanMessage, SystemMessage

api_key = os.environ.get("BESTAIPRICE_API_KEY")
if not api_key:
    print("Error: BESTAIPRICE_API_KEY environment variable is required.", file=sys.stderr)
    sys.exit(1)

base_url = os.environ.get("BESTAIPRICE_BASE_URL", "https://bestaiprice.com/gateway").rstrip("/") + "/v1"
model = os.environ.get("MODEL", "gpt-4o")

print(f"Initializing LangChain ChatOpenAI with base_url={base_url}, model={model}...")

llm = ChatOpenAI(
    api_key=api_key,
    base_url=base_url,
    model=model,
    temperature=0.3,
)

messages = [
    SystemMessage(content="You are an expert AI agent architect."),
    HumanMessage(content="Describe a resilient failover strategy for LLM API consumers in 2 bullet points."),
]

response = llm.invoke(messages)

print("\n--- LangChain Response ---")
print(response.content)
