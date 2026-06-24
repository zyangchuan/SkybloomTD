from __future__ import annotations

import logging
import queue
import threading
from typing import Any, Callable

from ocr_coordinator.config import (
    RABBITMQ_BLOCKED_CONNECTION_TIMEOUT_SECONDS,
    RABBITMQ_HEARTBEAT_SECONDS,
    RABBITMQ_URL,
    RABBITMQ_WORKER_POLL_SECONDS,
)

LOG = logging.getLogger(__name__)


def rabbitmq_parameters():
    import pika

    params = pika.URLParameters(RABBITMQ_URL)
    params.heartbeat = RABBITMQ_HEARTBEAT_SECONDS
    params.blocked_connection_timeout = RABBITMQ_BLOCKED_CONNECTION_TIMEOUT_SECONDS
    return params


def run_with_heartbeat_pump(
    connection,
    work_fn: Callable[[], dict[str, Any]],
    poll_seconds: float = RABBITMQ_WORKER_POLL_SECONDS,
) -> dict[str, Any]:
    results: queue.Queue[tuple[bool, dict[str, Any] | BaseException]] = queue.Queue(
        maxsize=1
    )

    def run_work() -> None:
        try:
            results.put((True, work_fn()))
        except BaseException as exc:
            results.put((False, exc))

    worker = threading.Thread(target=run_work, name="document-job-worker")
    worker.start()
    try:
        while worker.is_alive():
            connection.process_data_events(time_limit=max(0.1, poll_seconds))
    except BaseException:
        LOG.exception("rabbitmq connection failed while document job was running")
        worker.join()
        raise

    worker.join()
    succeeded, value = results.get()
    if succeeded:
        return value  # type: ignore[return-value]
    raise value
