# 📖 Руководство по Auth API (Аутентификация и Авторизация)

Данное руководство предназначено для интеграции внешнего фронтенда с бэкендом **sub2api**.

---

## 🔐 1. Общие правила работы с API

- **Базовый URL:** `https://<your-sub2api-domain>/api/v1`
- **Формат запросов/ответов:** `Content-Type: application/json`
- **Авторизационный заголовок:**
  ```http
  Authorization: Bearer <access_token>
  ```
- **Стандартная структура ответа:**
  ```json
  {
    "code": 0,
    "data": { ... },
    "message": "success"
  }
  ```
  *(Если `code != 0`, произошла ошибка. Сообщение об ошибке находится в поле `message`)*.

---

## ⚙️ 2. Получение настроек системы

Перед показом форм регистрации и входа рекомендуется запросить конфигурацию сервера:

### `GET /api/v1/settings/public`
- **Доступ:** Публичный
- **Ответ:**
```json
{
  "code": 0,
  "data": {
    "register_enabled": true,
    "email_verify_enabled": false,
    "turnstile_enabled": false,
    "turnstile_site_key": ""
  }
}
```

---

## ✉️ 3. Отправка кода подтверждения на Email

Используется, если на сервере включен флаг `email_verify_enabled = true`.

### `POST /api/v1/auth/send-verify-code`
- **Тело запроса:**
```json
{
  "email": "user@example.com",
  "turnstile_token": ""
}
```
- **Ответ:**
```json
{
  "code": 0,
  "data": {
    "message": "Verification code sent",
    "countdown": 60
  }
}
```

---

## 📝 4. Регистрация нового пользователя

### `POST /api/v1/auth/register`
- **Тело запроса:**
```json
{
  "email": "user@example.com",
  "password": "strongpassword123",
  "verify_code": "123456",
  "promo_code": "",
  "invitation_code": "",
  "turnstile_token": ""
}
```
- **Ответ:**
```json
{
  "code": 0,
  "data": {
    "access_token": "eyJhbGciOi...",
    "refresh_token": "rt_987654321...",
    "token_type": "Bearer",
    "expires_in": 86400,
    "user": {
      "id": 15,
      "email": "user@example.com",
      "role": "user",
      "status": "active",
      "created_at": "2026-08-18T13:40:00Z"
    }
  }
}
```

---

## 🔑 5. Вход (Логин)

### `POST /api/v1/auth/login`
- **Тело запроса:**
```json
{
  "email": "user@example.com",
  "password": "strongpassword123",
  "turnstile_token": ""
}
```

#### Вариант А: Обычный вход (2FA отключен)
```json
{
  "code": 0,
  "data": {
    "access_token": "eyJhbGciOi...",
    "refresh_token": "rt_abcdef12345...",
    "token_type": "Bearer",
    "expires_in": 86400,
    "user": {
      "id": 15,
      "email": "user@example.com",
      "role": "user",
      "status": "active"
    }
  }
}
```

#### Вариант Б: Требуется 2FA-код
Если у пользователя включена двухфакторная аутентификация:
```json
{
  "code": 0,
  "data": {
    "require_2fa": true,
    "temp_token": "tmp_token_12345..."
  }
}
```

---

## 🛡️ 6. Подтверждение 2FA при входе

### `POST /api/v1/auth/login/2fa`
- **Тело запроса:**
```json
{
  "temp_token": "tmp_token_12345...",
  "code": "123456"
}
```
- **Ответ:** Возвращает стандартный объект с `access_token`, `refresh_token` и `user`.

---

## 🔄 7. Обновление токена (Refresh Token)

Когда срок жизни `access_token` истекает, фронтенд обменивает `refresh_token` на новую пару токенов без необходимости повторного ввода пароля:

### `POST /api/v1/auth/refresh`
- **Тело запроса:**
```json
{
  "refresh_token": "rt_abcdef12345..."
}
```
- **Ответ:**
```json
{
  "code": 0,
  "data": {
    "access_token": "eyJhbGciOi...",
    "refresh_token": "rt_new_token_67890...",
    "token_type": "Bearer",
    "expires_in": 86400
  }
}
```

---

## 🚪 8. Выход (Logout)

### `POST /api/v1/auth/logout`
- **Заголовки:** `Authorization: Bearer <access_token>`
- **Тело запроса:**
```json
{
  "refresh_token": "rt_abcdef12345..."
}
```
- **Ответ:**
```json
{
  "code": 0,
  "message": "success"
}
```

---

## 👤 9. Профиль текущего пользователя

### `GET /api/v1/user/profile`
- **Заголовки:** `Authorization: Bearer <access_token>`
- **Ответ:**
```json
{
  "code": 0,
  "data": {
    "id": 15,
    "email": "user@example.com",
    "role": "user",
    "status": "active",
    "balance": 0.0,
    "created_at": "2026-08-18T13:40:00Z"
  }
}
```

---

## 🔄 10. Восстановление пароля

Требует включённого `password_reset_enabled` и настроенного SMTP. Флаг `email_verify_enabled`
(проверка почты при регистрации) для этого не нужен — ссылка сама уходит на адрес аккаунта.

### Шаг 1: Запрос письма со ссылкой
`POST /api/v1/auth/forgot-password`
```json
{
  "email": "user@example.com"
}
```
Ответ всегда одинаковый (защита от перебора аккаунтов). Если аккаунт существует, на почту
приходит ссылка `<frontend_url>/reset-password?email=...&token=...` (настройка `frontend_url`
обязательна, иначе API отвечает `500 Password reset is not configured`).

### Шаг 2: Установка нового пароля
`POST /api/v1/auth/reset-password`
```json
{
  "email": "user@example.com",
  "token": "token_from_email_link",
  "new_password": "new_strong_password"
}
```
Токен одноразовый и берётся из ссылки в письме; `verify_code` из письма при регистрации здесь
не используется. При успехе инкрементируется `TokenVersion`, то есть все выданные ранее JWT
становятся недействительными.

---

## 💻 11. Пример интеграции (TypeScript / JavaScript)

```typescript
const API_BASE = 'https://api.yourdomain.com/api/v1';

// 1. Авторизация
export async function login(email: string, password: string) {
  const res = await fetch(`${API_BASE}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password })
  });
  
  const result = await res.json();
  if (result.code !== 0) {
    throw new Error(result.message || 'Ошибка входа');
  }

  // Сохраняем токены
  localStorage.setItem('access_token', result.data.access_token);
  localStorage.setItem('refresh_token', result.data.refresh_token);
  return result.data.user;
}

// 2. Хелпер для выполнения авторизованных запросов
export async function authFetch(endpoint: string, options: RequestInit = {}) {
  let token = localStorage.getItem('access_token');
  
  const headers = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${token}`,
    ...options.headers,
  };

  let response = await fetch(`${API_BASE}${endpoint}`, { ...options, headers });

  // Если токен истек (401), пробуем обновить через refresh_token
  if (response.status === 401) {
    const refreshToken = localStorage.getItem('refresh_token');
    if (!refreshToken) throw new Error('Требуется авторизация');

    const refreshRes = await fetch(`${API_BASE}/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken })
    });
    
    const refreshData = await refreshRes.json();
    if (refreshData.code === 0) {
      token = refreshData.data.access_token;
      localStorage.setItem('access_token', token);
      localStorage.setItem('refresh_token', refreshData.data.refresh_token);
      
      // Повторяем исходный запрос с новым токеном
      headers['Authorization'] = `Bearer ${token}`;
      response = await fetch(`${API_BASE}${endpoint}`, { ...options, headers });
    } else {
      localStorage.removeItem('access_token');
      localStorage.removeItem('refresh_token');
      throw new Error('Сессия истекла, войдите снова');
    }
  }

  return response.json();
}
```
