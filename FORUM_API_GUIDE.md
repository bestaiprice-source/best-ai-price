# 💬 Руководство по Forum API (Сообщения, Ответы и Реакции)

Данное руководство описывает интеграцию единого API форума для публикации сообщений, древовидных ответов и реакций (лайков и дизлайков) во внешнем фронтенде.

---

## 📌 1. Основные правила и архитектура

- **Базовый URL:** `https://<your-sub2api-domain>/api/v1`
- **Аутентификация:** Для создания постов, ответов и реакций (лайков/дизлайков) требуется заголовок:
  ```http
  Authorization: Bearer <access_token>
  ```
- **Чтение сообщений и ответов:** Доступно как публично, так и авторизованным пользователям. При передаче `Authorization: Bearer <token>` в ответе для каждого сообщения автоматически возвращается поле `user_vote` (`"like"`, `"dislike"` или `""`), отражающее персональный голос текущего пользователя.
- **Древовидная вложенность:** Пользователь может написать корневой пост (`POST /api/v1/forum/posts`) или ответить на любой существующий пост/ответ (`POST /api/v1/forum/posts/:id/reply`), создавая ветки любой глубины.
- **Политика удаления:** Пользователи не могут удалять опубликованные посты/ответы во избежание разрушения цепочек дискуссий. Удалять/отменять можно только свой голос (`vote_type: "none"`).

---

## 📋 2. Список эндпоинтов

| Метод | Путь | Авторизация | Описание |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/v1/forum/posts` | Опционально | Список корневых постов с пагинацией и сортировкой |
| `POST` | `/api/v1/forum/posts` | Обязательно | Опубликовать новый корневой пост |
| `GET` | `/api/v1/forum/posts/:id/replies` | Опционально | Получить список ответов на конкретный пост/ответ |
| `POST` | `/api/v1/forum/posts/:id/reply` | Обязательно | Ответить на пост или другой ответ |
| `POST` | `/api/v1/forum/posts/:id/vote` | Обязательно | Поставить лайк, дизлайк или отменить голос |
| `DELETE`| `/api/v1/forum/posts/:id` | Обязательно | Запрещено (возвращает `403 Forbidden`) |
| `GET` | `/api/v1/admin/forum/posts` | Admin | Админский список всех постов с фильтрами |
| `PUT` | `/api/v1/admin/forum/posts/:id/status` | Admin | Смена статуса сообщения (`active`, `hidden`) |

---

## 🔍 3. Детальное описание запросов и ответов

### 1. Получение списка корневых постов

Возвращает список корневых постов форума с пагинацией и сортировкой.

- **`GET /api/v1/forum/posts`**
- **Query-параметры:**
  - `page` *(число, по умолчанию `1`)*: Номер страницы.
  - `page_size` *(число, по умолчанию `20`, макс `100`)*: Размер страницы.
  - `sort` *(строка, опционально)*:
    - `"latest"` (по умолчанию — сначала новые)
    - `"top_liked"` (сначала самые залайканные)
    - `"most_replies"` (с наибольшим числом ответов)

- **Пример запроса:**
  ```http
  GET /api/v1/forum/posts?page=1&page_size=20&sort=top_liked
  Authorization: Bearer <access_token>   <-- Опционально (для заполнения user_vote)
  ```

- **Пример ответа:**
  ```json
  {
    "code": 0,
    "data": {
      "posts": [
        {
          "id": 1,
          "user_id": 42,
          "user_email_masked": "u***r@example.com",
          "parent_id": null,
          "content": "Всем привет! Делитесь опытом использования новых моделей.",
          "status": "active",
          "like_count": 15,
          "dislike_count": 1,
          "reply_count": 4,
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

### 2. Публикация нового корневого поста

- **`POST /api/v1/forum/posts`**
- **Заголовки:**
  - `Content-Type: application/json`
  - `Authorization: Bearer <access_token>`
- **Тело запроса:**
  ```json
  {
    "content": "Текст вашего сообщения (до 5000 символов)"
  }
  ```
- **Ответ:**
  ```json
  {
    "code": 0,
    "data": {
      "id": 2,
      "user_id": 42,
      "user_email_masked": "u***r@example.com",
      "parent_id": null,
      "content": "Текст вашего сообщения (до 5000 символов)",
      "status": "active",
      "like_count": 0,
      "dislike_count": 0,
      "reply_count": 0,
      "user_vote": "",
      "created_at": "2026-09-02T14:05:00Z",
      "updated_at": "2026-09-02T14:05:00Z"
    }
  }
  ```

---

### 3. Получение ответов на пост или ответ

- **`GET /api/v1/forum/posts/:id/replies`**
- **Query-параметры:** `page` (по умолчанию `1`), `page_size` (по умолчанию `20`).
- **Ответ:**
  ```json
  {
    "code": 0,
    "data": {
      "post_id": 1,
      "replies": [
        {
          "id": 3,
          "user_id": 55,
          "user_email_masked": "m***e@test.com",
          "parent_id": 1,
          "content": "Отличная идея, поддерживаю!",
          "status": "active",
          "like_count": 3,
          "dislike_count": 0,
          "reply_count": 1,
          "user_vote": "like",
          "created_at": "2026-09-02T14:10:00Z",
          "updated_at": "2026-09-02T14:10:00Z"
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

### 4. Ответ на пост или ответ (Вложенный ответ)

- **`POST /api/v1/forum/posts/:id/reply`**
- **Заголовки:** `Authorization: Bearer <access_token>`
- **Тело запроса:**
  ```json
  {
    "content": "Полностью согласен!"
  }
  ```
- **Ответ:** возвращает созданный объект ответа с заполненным `parent_id`. У родительского поста счетчик `reply_count` увеличивается автоматически.

---

### 5. Реакции (Лайк, Дизлайк, Отмена реакции)

Один пользователь может иметь **только один активный голос** на каждое сообщение.

- **`POST /api/v1/forum/posts/:id/vote`**
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
      "post_id": 1,
      "like_count": 16,
      "dislike_count": 1,
      "user_vote": "like"
    }
  }
  ```

---

## 💻 4. Пример TypeScript-клиента

```typescript
const API_BASE = 'https://api.yourdomain.com/api/v1';

function getAuthHeader(): Record<string, string> {
  const token = localStorage.getItem('access_token');
  return token ? { 'Authorization': `Bearer ${token}` } : {};
}

export const ForumAPI = {
  // 1. Получить корневые посты
  async getPosts(page = 1, pageSize = 20, sort = 'latest') {
    const params = new URLSearchParams({
      page: page.toString(),
      page_size: pageSize.toString(),
      sort,
    });

    const res = await fetch(`${API_BASE}/forum/posts?${params}`, {
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      }
    });
    return (await res.json()).data;
  },

  // 2. Создать новый корневой пост
  async createPost(content: string) {
    const res = await fetch(`${API_BASE}/forum/posts`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      },
      body: JSON.stringify({ content })
    });
    return (await res.json()).data;
  },

  // 3. Получить ответы на пост
  async getReplies(postId: number, page = 1, pageSize = 20) {
    const params = new URLSearchParams({
      page: page.toString(),
      page_size: pageSize.toString()
    });

    const res = await fetch(`${API_BASE}/forum/posts/${postId}/replies?${params}`, {
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      }
    });
    return (await res.json()).data;
  },

  // 4. Ответить на пост/ответ
  async replyPost(postId: number, content: string) {
    const res = await fetch(`${API_BASE}/forum/posts/${postId}/reply`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...getAuthHeader()
      },
      body: JSON.stringify({ content })
    });
    return (await res.json()).data;
  },

  // 5. Поставить лайк / дизлайк / отменить
  async votePost(postId: number, voteType: 'like' | 'dislike' | 'none') {
    const res = await fetch(`${API_BASE}/forum/posts/${postId}/vote`, {
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
