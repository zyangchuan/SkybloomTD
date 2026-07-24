# User Service

Go Gin API for storing authenticated Supabase users in PostgreSQL.

## Endpoint

`POST /users` or `POST /users/me`

Cookie:

```http
skybloom_access_token=<supabase-jwt>
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

`GET /users/{id}` returns a stored user profile by Supabase user id.
