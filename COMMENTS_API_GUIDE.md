# 💬 Руководство по API Комментариев, Отзывов и Реакций

Данное руководство предназначено для интеграции системы отзывов и комментариев к AI-провайдерам (например, `yunwu`, `openai`, `claude` и др.) во внешнем фронтенде.

---

## 📌 1. Основные правила и архитектура

- **Базовый URL:** `https://<your-sub2api-domain>/api/v1`
- **Аутентификация:** Для создания комментариев, ответов и реакций (лайков/дизлайков) требуется заголовок:
  ```http
  Authorization: Bearer <access_token>
  ```
- **Тональность отзывов (Sentiment):** Поддерживаются значения `"positive"` (положительный) или `"negative"` (отрицательный). Звездный рейтинг 1-5 не используется.
- **Вложенность ответов:** Пользователи могут отвечать на отзывы других пользователей (создаются цепочки комментариев).
- **Политика удаления:** Пользователи **не могут удалять свои комментарии** после публикации. Удалять/отменять можно **только свою реакцию** (лайк или дизлайк).

---

## 📋 2. Список эндпоинтов

| Метод | Путь | Авторизация | Описание |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/v1/providers/:provider/comments` | Опционально | Список отзывов к провайдеру + сводная статистика |
| `POST` | `/api/v1/providers/:provider/comments` | Обязательно | Опубликовать корневой отзыв о провайдере |
| `GET` | `/api/v1/comments/:id/replies` | Опционально | Получить список ответов на конкретный комментарий |
| `POST` | `/api/v1/comments/:id/reply` | Обязательно | Ответить на комментарий другого пользователя |
| `POST` | `/api/v1/comments/:id/vote` | Обязательно | Поставить лайк, дизлайк или отменить реакцию |
| `DELETE`| `/api/v1/comments/:id` | Обязательно | Запрещено (возвращает 403 Forbidden) |

---

## 🔍 3. Детальное описание запросов и ответов

### 1. Получение отзывов и сводной статистики провайдера

Возвращает список корневых отзывов и аналитическую сводку (процент положительных отзывов, количество лайков/дизлайков).

- **`GET /api/v1/providers/:provider/comments`**
- **Query-параметры:**
  - `page` *(число, по умолчанию `1`)*: Номер страницы.
  - `page_size` *(число, по умолчанию `20`, макс `100`)*: Размер страницы.
  - `sentiment` *(строка, опционально)*: Фильтр по тональности (`"positive"` или `"negative"`).
  - `sort` *(строка, опционально)*: Сортировка:
    - `"latest"` (по умолчанию — сначала новые)
    - `"top_liked"` (сначала самые залайканные)
    - `"most_replies"` (с наибольшим числом ответов)

- **Пример запроса:**
  ```http
  GET /api/v1/providers/yunwu/comments?page=1&page_size=20&sort=top_liked
  Authorization: Bearer <access_token>   <-- Опционально (нужен для получения user_vote)
  ```

- **Пример ответа:**
  ```json
  {
    "code": 0,
    "data": {
      "provider": "yunwu",
      "summary": {
        "provider": "yunwu",
        "total_comments": 48,
        "positive_count": 42,
        "negative_count": 6,
        "positive_ratio": 87.5
      },
      "comments": [
        {
          "id": 1,
          "provider": "yunwu",
          "user_id": 105,
          "user_email_masked": "a***x@gmail.com",
          "parent_id": null,
          "content": "Очень стабильный провайдер, отличная скорость на Claude 3.5 Sonnet!",
          "sentiment": "positive",
          "status": "active",
          "like_count": 12,
          "dislike_count": 1,
          "reply_count": 4,
          "user_vote": "like",
          "created_at": "2026-08-18T14:20:00Z",
          "updated_at": "2026-08-18T14:20:00Z"
        }
      ],
      "pagination": {
        "page": 1,
        "page_size": 20,
        "total": 48,
        "pages": 3
      }
    }
  }
  ```

---

### 2. Публикация нового отзыва о провайдере

- **`POST /api/v1/providers/:provider/comments`**
- **Заголовки:**
  - `Content-Type: application/json`
  - `Authorization: Bearer <access_token>`
- **Тело запроса:**
  ```json
  {
    "content": "Генерация текста быстрая, но бывают очереди в пиковые часы.",
    "sentiment": "positive" // "positive" или "negative" (по умолчанию "positive")
  }
  ```
- **Ответ:**
  ```json
  {
    "code": 0,
    "data": {
      "id": 2,
      "provider": "yunwu",
      "user_id": 15,
      "user_email_masked": "u***r@example.com",
      "parent_id": null,
      "content": "Генерация текста быстрая, но бывают очереди в пиковые часы.",
      "sentiment": "positive",
      "status": "active",
      "like_count": 0,
      "dislike_count": 0,
      "reply_count": 0,
      "user_vote": "",
      "created_at": "2026-08-19T08:00:00Z",
      "updated_at": "2026-08-19T08:00:00Z"
    }
  }
  ```

---

### 3. Получение ответов на комментарий

- **`GET /api/v1/comments/:id/replies`**
- **Query-параметры:** `page`, `page_size`.
- **Ответ:**
  ```json
  {
    "code": 0,
    "data": {
      "comment_id": 1,
      "replies": [
        {
          "id": 5,
          "provider": "yunwu",
          "user_id": 120,
          "user_email_masked": "m***e@test.com",
          "parent_id": 1,
          "content": "Согласен, подтверждаю хорошую работу с большими контекстами.",
          "sentiment": "neutral",
          "status": "active",
          "like_count": 3,
          "dislike_count": 0,
          "reply_count": 0,
          "user_vote": "like",
          "created_at": "2026-08-19T08:15:00Z"
        }
      ],
      "pagination": {
        "page": 1,
        "page_size": 20,
        "total": 1,
        "pages": 1
      }
    }
  }
  ```

---

### 4. Ответ на комментарий (Вложенный ответ)

- **`POST /api/v1/comments/:id/reply`**
- **Заголовки:** `Authorization: Bearer <access_token>`
- **Тело запроса:**
  ```json
  {
    "content": "Полностью согласен с автором!"
  }
  ```
- **Ответ:** возвращает созданный объект комментария. У родительского комментария счетчик `reply_count` увеличивается автоматически.

---

### 5. Реакции (Лайк, Дизлайк, Отмена реакции)

Один пользователь может иметь **только один голос** на комментарий.

- **`POST /api/v1/comments/:id/vote`**
- **Заголовки:** `Authorization: Bearer <access_token>`
- **Варианты тела запроса:**
  1. Поставить лайк:
     ```json
     { "vote_type": "like" }
     ```
  2. Поставить дизлайк:
     ```json
     { "vote_type": "dislike" }
     ```
  3. Отменить голос (удалить лайк или дизлайк):
     ```json
     { "vote_type": "none" }
     ```
- **Ответ:**
  ```json
  {
    "code": 0,
    "data": {
      "comment_id": 1,
      "like_count": 13,
      "dislike_count": 1,
      "user_vote": "like" // текущий активный голос пользователя ("like", "dislike" или "none")
    }
  }
  ```

---

## 💻 4. Пример готового сервиса на TypeScript (Vue / React / Vanilla)

```typescript
const API_BASE = 'https://api.yourdomain.com/api/v1';

function getAuthHeader(): Record<string, string> {
  const token = localStorage.getItem('access_token');
  return token ? { 'Authorization': `Bearer ${token}` } : {};
}

export const ProviderCommentsAPI = {
  // 1. Получить отзывы провайдера и сводку
  async getComments(provider: string, page = 1, pageSize = 20, sort = 'latest', sentiment?: 'positive' | 'negative') {
    const params = new URLSearchParams({
      page: page.toString(),
      page_size: pageSize.toString(),
      sort,
      ...(sentiment ? { sentiment } : {})
    });

    const res = await fetch(`${API_BASE}/providers/${provider}/comments?${params}`, {
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      }
    });
    return (await res.json()).data;
  },

  // 2. Оставить отзыв
  async createReview(provider: string, content: string, sentiment: 'positive' | 'negative') {
    const res = await fetch(`${API_BASE}/providers/${provider}/comments`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      },
      body: JSON.stringify({ content, sentiment })
    });
    return await res.json();
  },

  // 3. Получить ответы на комментарий
  async getReplies(commentId: number, page = 1, pageSize = 20) {
    const res = await fetch(`${API_BASE}/comments/${commentId}/replies?page=${page}&page_size=${pageSize}`, {
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      }
    });
    return (await res.json()).data;
  },

  // 4. Ответить на комментарий
  async replyToComment(commentId: number, content: string) {
    const res = await fetch(`${API_BASE}/comments/${commentId}/reply`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      },
      body: JSON.stringify({ content })
    });
    return await res.json();
  },

  // 5. Поставить реакцию / отменить реакцию
  async vote(commentId: number, voteType: 'like' | 'dislike' | 'none') {
    const res = await fetch(`${API_BASE}/comments/${commentId}/vote`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      },
      body: JSON.stringify({ vote_type: voteType })
    });
    return (await res.json()).data;
  }
};
```
