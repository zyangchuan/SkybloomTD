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

Document uploads return a durable `document_id`, the main `task_id`, and
`is_ready: false`. Clients can poll:

```text
GET /api/document-content/tasks/{task_id}/status
```

for temporary Redis task status while the document is being processed.

Clients can also list the authenticated user's documents:

```text
GET /api/document-content/documents
```
