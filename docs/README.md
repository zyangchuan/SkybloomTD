# API Documentation

The public API contract is in `docs/openapi.yaml`.

When the Docker stack is running, Nginx also serves it at:

```text
http://localhost/openapi.yaml
```

Swagger UI is available at:

```text
http://localhost/docs
```

All browser-facing API calls should go through the reverse proxy on port 80.
Backend containers are private Docker-network services.

Protected API routes use the `skybloom_access_token` cookie. The browser sends
it automatically to the reverse proxy, Nginx verifies it through user-service,
and private services receive trusted user headers.

Document uploads require multipart `file` and `game_name` fields. They return
a durable `document_id`, the stored `game_name`, the main `task_id`, and
`is_ready: false`. Clients can poll:

```text
GET /api/document-content/tasks/{task_id}/status
```

for temporary Redis task status while the document is being processed.

Clients can also list the authenticated user's documents:

```text
GET /api/document-content/documents
```

To delete a document and its generated content:

```text
DELETE /api/document-content/documents/{document_id}
```

To browse indexed document structure:

```text
GET /api/document-content/documents/{document_id}/chapters
GET /api/document-content/chapters/{chapter_id}/sub-chapters
```

## Game Service

The game client connects to the authoritative websocket endpoint:

```text
ws://localhost/api/game-service/ws
```

After connecting, start or reuse level generation by sending:

```json
{
  "type": "game.start",
  "data": {
    "sub_chapter_id": "00000000-0000-0000-0000-000000000000"
  }
}
```

The server returns `level_generation.started` with a `generation_id`,
`status_url`, map seed/version, per-step statuses, and a `reused` flag. The
frontend then polls:

```text
GET /api/game-service/level-generation/{generation_id}/status
```

When `status` is `complete`, use the returned `level_id` to load the game over
the same websocket:

```json
{
  "type": "game.load",
  "data": {
    "level_id": "00000000-0000-0000-0000-000000000000"
  }
}
```

The server replies with `game.initial_state`, which contains only the generated
18x12 map, enemy path tile metadata, and object locations. Objects are generated
outside the path's one-tile placement buffer. Quiz answers stay on the backend
in `game-redis` for authoritative validation during play.

To begin the live game loop after the map is loaded, send:

```json
{
  "type": "game.session.start",
  "data": {
    "level_id": "00000000-0000-0000-0000-000000000000"
  }
}
```

The server creates a Redis-backed gameplay session with `health=100`,
`essence=1000`, and `wave=0`, replies with `game.session.started`, then streams
`game.state` snapshots over the websocket at 20 updates per second. The session
start payload includes the available bird types, stats, and attack modes.
Redis is used for the session checkpoint; the hot 20 Hz loop runs in the
game-service process. Reconnecting with the same `level_id` reuses the existing
Redis session for that user and level, including placed birds and current
essence, until the session expires.

To place a tower, send the bird type and grid position:

```json
{
  "type": "game.action.place_tower",
  "data": {
    "bird_type": "sparrow",
    "x": 4,
    "y": 7
  }
}
```

The server validates the placement, consumes the bird cost from essence,
persists the placed bird snapshot to Redis, and responds with
`game.action.accepted` or `game.action.rejected`.

Level generation is idempotent per user, sub-chapter, and map algorithm version.
If the database already has quizzes for that user's sub-chapter, the game
service reuses the saved level and skips quiz generation. Quizzes are persisted
in PostgreSQL and copied into the dedicated `game-redis` container for in-game
answer validation. Generated map data is also cached in `game-redis` and can be
regenerated from the stored seed if the cache expires.
