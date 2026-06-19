import json
import logging
from datetime import datetime, timezone

import redis 

from .config import REDIS_URL, TASK_STATUS_TTL_SECONDS


LOG = logging.getLogger(__name__)
_client: redis.Redis | None = None


def redis_client() -> redis.Redis | None:
    global _client

    if not REDIS_URL:
        return None
    if _client is None:
        _client = redis.Redis.from_url(REDIS_URL, decode_responses=True)
    return _client


def set_task_status(
    task_id: str | None,
    document_id: str | None,
    status: str,
    error: str | None = None,
) -> None:
    if not task_id:
        return

    client = redis_client()
    if client is None:
        LOG.debug("redis task status skipped because REDIS_URL is not configured")
        return

    payload = {
        "task_id": task_id,
        "document_id": document_id,
        "status": status,
        "error": error,
        "updated_at": datetime.now(timezone.utc).isoformat(),
    }

    try:
        client.set(
            f"task:{task_id}",
            json.dumps(payload),
            ex=TASK_STATUS_TTL_SECONDS,
        )
    except Exception:
        LOG.exception("failed to write redis task status task_id=%s", task_id)
