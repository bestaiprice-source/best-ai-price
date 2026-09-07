# 💬 Руководство по Forum API (Топики, Комментарии и Реакции)

Данное руководство описывает интеграцию единого API форума для создания тематических обсуждений (**Топиков**), комментариев к ним, древовидных ответов и реакций (лайков и дизлайков) во внешнем фронтенде.

---

## 📌 1. Основные правила и архитектура

- **Базовый URL:** `https://<your-sub2api-domain>/api/v1`
- **Аутентификация:** Для создания топиков, комментариев, ответов и реакций (лайков/дизлайков) требуется заголовок:
  ```http
  Authorization: Bearer <access_token>
  ```
- **Чтение топиков и комментариев:** Доступно как публично, так и авторизованным пользователям. При передаче `Authorization: Bearer <token>` в ответе для каждого топика и комментария автоматически возвращается поле `user_vote` (`"like"`, `"dislike"` или `""`), отражающее персональный голос текущего пользователя.
- **Топики (Topics):** Имеют заголовок (`title`, до 255 символов) и описание (`content`, до 10000 символов).
- **Комментарии и ответы (Comments & Replies):** Комментарии привязаны к конкретному топику (`POST /api/v1/forum/topics/:id/comments`). На любой комментарий можно ответить (`POST /api/v1/forum/comments/:id/reply`), создавая вложенные ветки ответов.
- **Политика удаления:** Пользователи не могут удалять опубликованные топики и комментарии во избежание разрушения цепочек дискуссий. Удалять/отменять можно только свой голос (`vote_type: "none"`).

---

## 📋 2. Список эндпоинтов

| Метод | Путь | Авторизация | Описание |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/v1/forum/topics` | Опционально | Список топиков с пагинацией, поиском и сортировкой |
| `POST` | `/api/v1/forum/topics` | Обязательно | Создать новый топик (`title`, `content`) |
| `GET` | `/api/v1/forum/topics/:id` | Опционально | Получить топик по ID |
| `POST` | `/api/v1/forum/topics/:id/vote` | Обязательно | Поставить реакцию на топик (`like`, `dislike`, `none`) |
| `DELETE`| `/api/v1/forum/topics/:id` | Обязательно | Запрещено (возвращает `403 Forbidden`) |
| `GET` | `/api/v1/forum/topics/:id/comments` | Опционально | Список корневых комментариев к топику |
| `POST` | `/api/v1/forum/topics/:id/comments` | Обязательно | Оставить корневой комментарий к топику |
| `GET` | `/api/v1/forum/comments/:id/replies` | Опционально | Список ответов на конкретный комментарий |
| `POST` | `/api/v1/forum/comments/:id/reply` | Обязательно | Ответить на комментарий |
| `POST` | `/api/v1/forum/comments/:id/vote` | Обязательно | Поставить реакцию на комментарий (`like`, `dislike`, `none`) |
| `DELETE`| `/api/v1/forum/comments/:id` | Обязательно | Запрещено (возвращает `403 Forbidden`) |
| `GET` | `/api/v1/admin/forum/topics` | Admin | Админский список топиков с фильтром по статусу |
| `PUT` | `/api/v1/admin/forum/topics/:id/status` | Admin | Смена статуса топика (`active`, `hidden`) |
| `GET` | `/api/v1/admin/forum/comments` | Admin | Админский список комментариев с фильтром по статусу |
| `PUT` | `/api/v1/admin/forum/comments/:id/status` | Admin | Смена статуса комментария (`active`, `hidden`) |

---

## 🔍 3. Детальное описание запросов и ответов

### 1. Получение списка топиков

Возвращает список топиков форума с пагинацией, поиском и сортировкой.

- **`GET /api/v1/forum/topics`**
- **Query-параметры:**
  - `page` *(число, по умолчанию `1`)*: Номер страницы.
  - `page_size` *(число, по умолчанию `20`, макс `100`)*: Размер страницы.
  - `search` *(строка, опционально)*: Поиск по заголовку топика.
  - `sort` *(строка, опционально)*:
    - `"latest"` (по умолчанию — сначала новые)
    - `"top_liked"` (сначала самые залайканные)
    - `"most_comments"` (с наибольшим числом комментариев)

- **Пример ответа:**
  ```json
  {
    "code": 0,
    "data": {
      "topics": [
        {
          "id": 1,
          "user_id": 42,
          "user_email_masked": "u***r@example.com",
          "title": "Как оптимизировать промпты для Claude 3.7 Sonnet?",
          "content": "Делитесь своими лучшими системными инструкциями и примерами использования thinking mode.",
          "status": "active",
          "like_count": 24,
          "dislike_count": 1,
          "comment_count": 8,
          "user_vote": "like",
          "created_at": "2026-09-02T14:00:00Z",
          "updated_at": "2026-09-02T14:00:00Z"
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

### 2. Создание топика

- **`POST /api/v1/forum/topics`**
- **Заголовки:**
  - `Content-Type: application/json`
  - `Authorization: Bearer <access_token>`
- **Тело запроса:**
  ```json
  {
    "title": "Заголовок темы (до 255 символов)",
    "content": "Описание темы и подробности (до 10000 символов)"
  }
  ```

---

### 3. Получение топика по ID

- **`GET /api/v1/forum/topics/:id`**
- **Ответ:** возвращает объект топика и текущий `user_vote`.

---

### 4. Реакция на топик (Лайк / Дизлайк / Отмена)

- **`POST /api/v1/forum/topics/:id/vote`**
- **Тело запроса:**
  - Лайк: `{ "vote_type": "like" }`
  - Дизлайк: `{ "vote_type": "dislike" }`
  - Отмена голоса: `{ "vote_type": "none" }`
- **Ответ:**
  ```json
  {
    "code": 0,
    "data": {
      "id": 1,
      "like_count": 25,
      "dislike_count": 1,
      "user_vote": "like"
    }
  }
  ```

---

### 5. Получение комментариев к топику

- **`GET /api/v1/forum/topics/:id/comments`**
- **Query-параметры:** `page` (по умолчанию `1`), `page_size` (по умолчанию `20`), `sort` (`latest`, `oldest`, `top_liked`).
- **Ответ:**
  ```json
  {
    "code": 0,
    "data": {
      "topic_id": 1,
      "comments": [
        {
          "id": 10,
          "topic_id": 1,
          "user_id": 55,
          "user_email_masked": "m***e@test.com",
          "parent_id": null,
          "content": "Я использую thinking budget 4000 токенов, качество отличное!",
          "status": "active",
          "like_count": 5,
          "dislike_count": 0,
          "reply_count": 2,
          "user_vote": "like",
          "created_at": "2026-09-02T14:15:00Z",
          "updated_at": "2026-09-02T14:15:00Z"
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

### 6. Добавление комментария к топику

- **`POST /api/v1/forum/topics/:id/comments`**
- **Заголовки:** `Authorization: Bearer <access_token>`
- **Тело запроса:**
  ```json
  {
    "content": "Текст комментария (до 5000 символов)"
  }
  ```

---

### 7. Ответ на комментарий (Вложенный ответ)

- **`POST /api/v1/forum/comments/:id/reply`**
- **Заголовки:** `Authorization: Bearer <access_token>`
- **Тело запроса:**
  ```json
  {
    "content": "Полностью поддерживаю предыдущего комментатора!"
  }
  ```
- **Получение списка ответов на комментарий:**
  - **`GET /api/v1/forum/comments/:id/replies?page=1&page_size=20`**

---

### 8. Реакция на комментарий (Лайк / Дизлайк / Отмена)

- **`POST /api/v1/forum/comments/:id/vote`**
- **Тело запроса:**
  - `{ "vote_type": "like" }`
  - `{ "vote_type": "dislike" }`
  - `{ "vote_type": "none" }`

---

## 💻 4. Пример TypeScript-клиента

```typescript
const API_BASE = 'https://api.yourdomain.com/api/v1';

function getAuthHeader(): Record<string, string> {
  const token = localStorage.getItem('access_token');
  return token ? { 'Authorization': `Bearer ${token}` } : {};
}

export const ForumAPI = {
  // 1. Получить список топиков
  async getTopics(page = 1, pageSize = 20, sort = 'latest', search = '') {
    const params = new URLSearchParams({
      page: page.toString(),
      page_size: pageSize.toString(),
      sort,
    });
    if (search) params.append('search', search);

    const res = await fetch(`${API_BASE}/forum/topics?${params}`, {
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      }
    });
    return (await res.json()).data;
  },

  // 2. Создать топик
  async createTopic(title: string, content: string) {
    const res = await fetch(`${API_BASE}/forum/topics`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      },
      body: JSON.stringify({ title, content })
    });
    return (await res.json()).data;
  },

  // 3. Получить топик по ID
  async getTopic(topicId: number) {
    const res = await fetch(`${API_BASE}/forum/topics/${topicId}`, {
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      }
    });
    return (await res.json()).data;
  },

  // 4. Проголосовать за топик
  async voteTopic(topicId: number, voteType: 'like' | 'dislike' | 'none') {
    const res = await fetch(`${API_BASE}/forum/topics/${topicId}/vote`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      },
      body: JSON.stringify({ vote_type: voteType })
    });
    return (await res.json()).data;
  },

  // 5. Получить комментарии топика
  async getComments(topicId: number, page = 1, pageSize = 20, sort = 'latest') {
    const params = new URLSearchParams({
      page: page.toString(),
      page_size: pageSize.toString(),
      sort
    });

    const res = await fetch(`${API_BASE}/forum/topics/${topicId}/comments?${params}`, {
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      }
    });
    return (await res.json()).data;
  },

  // 6. Добавить комментарий к топику
  async createComment(topicId: number, content: string) {
    const res = await fetch(`${API_BASE}/forum/topics/${topicId}/comments`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      },
      body: JSON.stringify({ content })
    });
    return (await res.json()).data;
  },

  // 7. Получить ответы на комментарий
  async getReplies(commentId: number, page = 1, pageSize = 20) {
    const params = new URLSearchParams({
      page: page.toString(),
      page_size: pageSize.toString()
    });

    const res = await fetch(`${API_BASE}/forum/comments/${commentId}/replies?${params}`, {
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      }
    });
    return (await res.json()).data;
  },

  // 8. Ответить на комментарий
  async replyComment(commentId: number, content: string) {
    const res = await fetch(`${API_BASE}/forum/comments/${commentId}/reply`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      },
      body: JSON.stringify({ content })
    });
    return (await res.json()).data;
  },

  // 9. Проголосовать за комментарий
  async voteComment(commentId: number, voteType: 'like' | 'dislike' | 'none') {
    const res = await fetch(`${API_BASE}/forum/comments/${commentId}/vote`, {
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
