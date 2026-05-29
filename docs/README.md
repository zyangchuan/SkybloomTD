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
`essence=1000`, `wave=0`, `loop_started=false`, and `loop_paused=false`, then
replies with `game.session.started`. The session start payload includes the
available bird types, stats, attack modes, restored birds, restored smogs, and
restored projectiles. The 20 Hz gameplay ticker starts only after the player
answers a quiz correctly; the `game.quiz.result` payload includes
`loop_started=true` when that happens. Redis is used for the session
checkpoint. Reconnecting with the same `level_id` reuses the existing Redis
session for that user and level, including placed birds, active enemies, active
projectiles, health, wave, tick, current essence, whether the loop has started,
and whether the loop is paused, until the session expires.

The gameplay loop currently runs three smog waves. Each wave is split into
three subwaves of grouped smog spawns, with larger gaps between individual
smogs and a pause between subwaves. The first wave starts on the first gameplay
tick after the loop is unlocked, and each later wave starts three seconds after
the previous wave has been cleared. Smogs move along the generated enemy path,
which avoids the top four and bottom four map rows so UI overlays do not cover
the path. Each smog that reaches the end removes 10 health. When health reaches
0, the loop stops and the server sends `game.over`. When all three waves are
cleared with health remaining, the loop stops and the server sends
`game.victory`. `game.state` includes active smog positions and health, active
projectiles, and transient combat events such as `bird.attack`, `smog.damage`,
`smog.spawned`, `smog.escaped`, and `wave.cleared`.

To pause a running game, send `game.pause`. The server persists
`loop_paused=true`, stops advancing ticks, enemies, combat, and wave spawning,
then replies with `game.paused`. Paused state is also included in
`game.session.started` and `game.state` as `loop_paused`.

To exit a game, send `game.exit`. The server stops the active loop, deletes the
Redis game session, user/level session index, and level quiz cache, then
replies with `game.exited`. This lets play-again repopulate the full quiz set
from PostgreSQL. If the loop has already stopped after victory or defeat,
include the `session_id` from `game.session.started` in the `game.exit` data.
Map cache entries are not deleted; they continue to expire by TTL.

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

To fetch a quiz during a running session, send:

```json
{
  "type": "game.quiz.request"
}
```

The server responds with `game.quiz.presented` containing only `quiz_id`,
`quiz_type`, `question_markdown`, and `options_markdown`. Submit the selected
zero-based option index with `game.quiz.answer`. The server validates the
answer, deletes that quiz from Redis, and responds with `game.quiz.result`.
Correct answers award 30 essence and unlock the gameplay loop if it has not
started yet. Incorrect answers are saved with the selected wrong option for a
later mistakes summary.

To retrieve the saved mistake summary for a completed or in-progress level, the
frontend can call the game-service HTTP API:

```text
GET /api/game-service/quiz-mistakes?level_id=00000000-0000-0000-0000-000000000000
```

The response includes only mistakes for the authenticated user and requested
level, including the question, options, selected option, and correct option.

Level generation is idempotent per user, sub-chapter, and map algorithm version.
If the database already has quizzes for that user's sub-chapter, the game
service reuses the saved level and skips quiz generation. Quizzes are persisted
in PostgreSQL and copied into the dedicated `game-redis` container for in-game
answer validation. New quiz generations request exactly 30 quizzes. Generated
map data is also cached in `game-redis` and can be regenerated from the stored
seed if the cache expires.
