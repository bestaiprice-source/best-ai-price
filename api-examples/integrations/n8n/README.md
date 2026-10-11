# n8n Workflow Automation — BestAIPrice

Integrate BestAIPrice with [n8n](https://n8n.io/) automation workflows using either the standard **OpenAI** or **Anthropic** nodes.

---

## 🛠️ Step 1: Configure Credentials in n8n

### For OpenAI Model Nodes:
1. In n8n, create a new Credential: **OpenAI API**.
2. **API Key**: Enter your BestAIPrice key (`sk-bestai-...`).
3. Click **Ignore SSL Issues**: (Leave False).
4. In any node that uses this credential (e.g., *OpenAI Chat Model*, *AI Agent*):
   - Set **Base URL**: `https://api.bestaiprice.com/v1`
   - Set **Model**: Enter model name (e.g. `gpt-4o`, `deepseek-chat`, `claude-3-7-sonnet`).

---

### For Anthropic Model Nodes:
1. In n8n, create an **Anthropic API** Credential.
2. **API Key**: Enter your BestAIPrice key (`sk-bestai-...`).
3. In the Anthropic node:
   - Override Base URL (if exposed) to `https://api.bestaiprice.com`.
   - Or use the **HTTP Request** node calling `https://api.bestaiprice.com/v1/messages`.

---

## 📦 Step 2: Import Pre-Built Workflow

Import [`n8n-workflow-bestaiprice.json`](./n8n-workflow-bestaiprice.json) directly into n8n:
- Open n8n canvas.
- Click `...` (Workflow menu) → **Import from File**.
- Select `n8n-workflow-bestaiprice.json`.
- Execute test trigger!
