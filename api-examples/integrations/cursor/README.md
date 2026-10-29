# Cursor IDE Integration — BestAIPrice

Configure [Cursor IDE](https://www.cursor.com/) to route all agent suggestions, composer tasks, and code completions through BestAIPrice.

---

## 🛠️ Step-by-Step Configuration

1. In Cursor, open **Settings** (`Ctrl+,` or `Cmd+,`), then click **Cursor Settings**.
2. Navigate to **Models**.
3. Under **OpenAI API Key**:
   - Turn **ON** `Override OpenAI Base URL`.
   - Set **Base URL**:
     ```text
     https://bestaiprice.com/gateway/v1
     ```
   - Enter your **API Key**:
     ```text
     sk-bestai-xxxxxxxxxxxxxxxxxxxx
     ```
4. Click **Verify**. Cursor will ping BestAIPrice's `/v1/models` and confirm connectivity.
5. In the **Model Names** list, enable the models you wish to use:
   - `claude-sonnet-5`
   - `claude-opus-5`
   - `gpt-4o`
   - `gpt-4o-mini`
   - `deepseek-v4-flash`
   - `deepseek-v4-pro`

---

## 📋 Recommended `.cursorrules`

Copy the provided [`.cursorrules`](./.cursorrules) file into the root of your project to automatically enforce:
- Low-latency concise responses
- Efficient token consumption
- Defensive coding practices
