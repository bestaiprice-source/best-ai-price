# 🔐 Руководство по API управления `auth_providers.json`

Данное руководство описывает работу с защищённым административным API для чтения и обновления файла `/data/auth_providers.json`.

---

## 🛡️ 1. Безопасность и правила доступа

- **Приватность:** Файл `auth_providers.json` **не доступен публично** и не раздаётся через веб-сервер без авторизации.
- **Требуемые права:** Доступ разрешён только администраторам.
- **Способы авторизации (выберите один):**
  1. Заголовок **`x-api-key: <admin-api-key>`** *(рекомендуется для скриптов и парсеров)*.
  2. Заголовок **`Authorization: Bearer <admin-jwt-token>`** *(для администраторов через веб-интерфейс)*.
- **Формат данных:** `Content-Type: application/json`.

---

## 📍 2. Доступные эндпоинты

| Метод | URL | Описание |
| :--- | :--- | :--- |
| `POST` / `PUT` | `/api/v1/admin/data/auth-providers` | Записать/перезаписать файл `auth_providers.json` |
| `GET` | `/api/v1/admin/data/auth-providers` | Прочитать текущее содержимое `auth_providers.json` |
| `POST` / `PUT` | `/api/v1/admin/auth-providers` | Алиас для записи |
| `GET` | `/api/v1/admin/auth-providers` | Алиас для чтения |
| `POST` / `PUT` | `/data/auth_providers.json` | Прямой защищённый эндпоинт записи |

---

## ✍️ 3. Запись данных (`POST` / `PUT`)

### Запрос:
- **URL:** `https://<your-domain>/gateway/api/v1/admin/data/auth-providers`
- **Заголовки:**
  ```http
  x-api-key: admin-3b9023fee06bcba60e613af485dfdc9e775d392453c647ba89a97afa3782b055
  Content-Type: application/json
  ```
- **Тело запроса (JSON):**
```json
{
  "version": "1.0",
  "updated_at": "2026-08-19T13:00:00Z",
  "providers": [
    {
      "id": "openai",
      "name": "OpenAI",
      "auth_type": "api_key",
      "auth_url": "https://platform.openai.com/api-keys",
      "doc_url": "https://platform.openai.com/docs",
      "models_count": 15,
      "status": "active"
    },
    {
      "id": "anthropic",
      "name": "Anthropic (Claude)",
      "auth_type": "oauth",
      "auth_url": "https://console.anthropic.com",
      "doc_url": "https://docs.anthropic.com",
      "models_count": 8,
      "status": "active"
    },
    {
      "id": "google",
      "name": "Google Gemini",
      "auth_type": "oauth",
      "auth_url": "https://aistudio.google.com",
      "doc_url": "https://ai.google.dev/docs",
      "models_count": 12,
      "status": "active"
    },
    {
      "id": "deepseek",
      "name": "DeepSeek",
      "auth_type": "api_key",
      "auth_url": "https://platform.deepseek.com",
      "doc_url": "https://platform.deepseek.com/api-docs",
      "models_count": 6,
      "status": "active"
    }
  ]
}
```

### Успешный ответ (`200 OK`):
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "bytes_written": 1160,
    "message": "auth_providers.json saved successfully"
  }
}
```

---

## 📖 4. Чтение данных (`GET`)

### Запрос:
- **URL:** `https://<your-domain>/gateway/api/v1/admin/data/auth-providers`
- **Заголовки:**
  ```http
  x-api-key: admin-3b9023fee06bcba60e613af485dfdc9e775d392453c647ba89a97afa3782b055
  ```

### Успешный ответ (`200 OK`):
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "version": "1.0",
    "updated_at": "2026-08-19T13:00:00Z",
    "providers": [
      {
        "auth_type": "api_key",
        "auth_url": "https://platform.openai.com/api-keys",
        "doc_url": "https://platform.openai.com/docs",
        "id": "openai",
        "models_count": 15,
        "name": "OpenAI",
        "status": "active"
      }
    ]
  }
}
```

---

## 💻 5. Примеры интеграции

### 1. cURL
```bash
# Запись
curl -X POST https://bestaiprice.com/gateway/api/v1/admin/data/auth-providers \
  -H "x-api-key: admin-3b9023fee06bcba60e613af485dfdc9e775d392453c647ba89a97afa3782b055" \
  -H "Content-Type: application/json" \
  -d '{"providers":[{"id":"openai","name":"OpenAI"}]}'

# Чтение
curl https://bestaiprice.com/gateway/api/v1/admin/data/auth-providers \
  -H "x-api-key: admin-3b9023fee06bcba60e613af485dfdc9e775d392453c647ba89a97afa3782b055"
```

### 2. Python (requests)
```python
import requests

API_URL = "https://bestaiprice.com/gateway/api/v1/admin/data/auth-providers"
ADMIN_KEY = "admin-3b9023fee06bcba60e613af485dfdc9e775d392453c647ba89a97afa3782b055"

headers = {
    "x-api-key": ADMIN_KEY,
    "Content-Type": "application/json"
}

# Отправка / обновление данных
payload = {
    "version": "1.0",
    "providers": [
        {"id": "openai", "name": "OpenAI", "auth_type": "api_key"},
        {"id": "anthropic", "name": "Anthropic Claude", "auth_type": "oauth"}
    ]
}

response = requests.post(API_URL, json=payload, headers=headers)
print("Save response:", response.status_code, response.json())

# Получение данных
response = requests.get(API_URL, headers=headers)
print("Get response:", response.status_code, response.json())
```

### 3. Node.js / TypeScript (fetch)
```typescript
const API_URL = 'https://bestaiprice.com/gateway/api/v1/admin/data/auth-providers';
const ADMIN_KEY = 'admin-3b9023fee06bcba60e613af485dfdc9e775d392453c647ba89a97afa3782b055';

// Запись данных
async function saveAuthProviders(data: any) {
  const res = await fetch(API_URL, {
    method: 'POST',
    headers: {
      'x-api-key': ADMIN_KEY,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(data)
  });
  return res.json();
}

// Чтение данных
async function getAuthProviders() {
  const res = await fetch(API_URL, {
    method: 'GET',
    headers: {
      'x-api-key': ADMIN_KEY
    }
  });
  return res.json();
}
```

---

## ⚠️ 6. Возможные ошибки

| HTTP Код | Код ошибки | Причина |
| :--- | :--- | :--- |
| `401 Unauthorized` | `UNAUTHORIZED` | Заголовок `x-api-key` или токен не передан |
| `401 Unauthorized` | `INVALID_ADMIN_KEY` | Передан неверный ключ (например, обычный user API ключ) |
| `400 Bad Request` | `BAD_REQUEST` | Тело запроса не является валидным JSON |
| `404 Not Found` | `NOT_FOUND` | Файл `auth_providers.json` ещё не был создан (при `GET`) |
