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
