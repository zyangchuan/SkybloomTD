# User Service

Go Gin API for storing authenticated Supabase users in PostgreSQL.

## Endpoint

`POST /users` or `POST /users/me`

Headers:

```http
Authorization: Bearer <supabase-jwt>
Content-Type: application/json
```

Body:

```json
{
  "user_name": "Ada",
  "email": "ada@example.com",
  "metadata": {
    "avatar_url": "https://example.com/avatar.png"
  }
}
```

The service stores the Supabase JWT `sub` claim as `private.users.id`.
