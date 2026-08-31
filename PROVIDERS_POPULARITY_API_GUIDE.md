# 🔥 Руководство по API кликов, популярности и анти-абуза AI-провайдеров

Данное руководство описывает систему учета кликов по AI-провайдерам (например, `openai`, `claude`, `gemini`, `antigravity`, `deepseek`, `yunwu` и др.), агрегации популярности и защиты от накрутки (Anti-Abuse Engine).

---

## 📌 1. Основные принципы и защита от накрутки (Anti-Abuse)

Для предотвращения бот-трафика, клик-спама и манипуляций с популярностью реализован многоуровневый фильтр:

1. **Deduplication Cooldown Window (Окно дедупликации)**:
   - Повторные клики по одному и тому же провайдеру с одного IP-адреса или одного аккаунта (`user_id`) в течение **15 минут** дедуплицируются.
   - Сервер возвращает статус `200 OK` с полем `counted: false` и `cooldown: true` без увеличения значения в БД.
2. **Global IP Rate Limiter (Ограничение частоты кликов с IP)**:
   - Лимит не более **30 кликов в минуту** с одного IP-адреса по всем провайдерам суммарно. При превышении возвращается `429 Too Many Requests`.
3. **Bot & Crawler Filter (Фильтрация ботов)**:
   - Запросы с отсутствующим или подозрительным `User-Agent` (`curl`, `wget`, `python-requests`, `Postman`, поисковые краулеры) автоматически отклоняются со статусом `403 Forbidden`.
4. **Атомарный инкремент в БД**:
   - Обновление счетчиков происходит через атомарный `UPSERT` (`ON CONFLICT (provider) DO UPDATE SET click_count = click_count + 1`).

---

## 📋 2. Список эндпоинтов

| Метод | Путь | Авторизация | Описание |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/providers/:provider/click` | Опционально | Зафиксировать клик/посещение провайдера с защитой от накрутки |
| `GET` | `/api/v1/providers/popularity` | Публично | Получить список всех провайдеров, отсортированный по популярности |
| `GET` | `/api/v1/providers/:provider/popularity` | Публично | Получить количество кликов конкретного провайдера |

---

## 🔍 3. Детальное описание запросов и ответов

### 1. Фиксация клика по провайдеру

- **`POST /api/v1/providers/:provider/click`**
- **Заголовки:**
  - `User-Agent` (обязателен, реальный браузер)
  - `Authorization: Bearer <token>` *(опционально — если пользователь авторизован)*

- **Пример ответа (первый клик / засчитан):**
  ```json
  {
    "code": 0,
    "message": "success",
    "data": {
      "provider": "openai",
      "click_count": 142,
      "counted": true,
      "cooldown": false
    }
  }
  ```

- **Пример ответа (повторный клик в течение 15 минут / кулдаун):**
  ```json
  {
    "code": 0,
    "message": "success",
    "data": {
      "provider": "openai",
      "click_count": 142,
      "counted": false,
      "cooldown": true
    }
  }
  ```

---

### 2. Получение общего рейтинга популярности

- **`GET /api/v1/providers/popularity`**

- **Пример ответа:**
  ```json
  {
    "code": 0,
    "message": "success",
    "data": {
      "providers": [
        {
          "provider": "claude",
          "click_count": 1250,
          "rank": 1,
          "last_clicked_at": "2026-08-31T14:30:00Z"
        },
        {
          "provider": "openai",
          "click_count": 980,
          "rank": 2,
          "last_clicked_at": "2026-08-31T14:25:00Z"
        },
        {
          "provider": "gemini",
          "click_count": 640,
          "rank": 3,
          "last_clicked_at": "2026-08-31T14:10:00Z"
        }
      ]
    }
  }
  ```

---

## 💻 4. Пример интеграции на фронтенде (Vue 3 / TypeScript)

```typescript
import { useProviderPopularity } from '@/composables/useProviderPopularity'

const { popularityMap, getClickCount, trackClick, sortByPopularity } = useProviderPopularity()

// При клике на карточку провайдера:
async function onSelectProvider(providerId: string) {
  // Локальный дебаунс + оптимистичное обновление счетчика
  await trackClick(providerId)
  window.open(`https://example.com/provider/${providerId}`, '_blank')
}

// Сортировка списка провайдеров по популярности:
const sortedList = sortByPopularity(providers, (p) => p.id)
```
