from __future__ import annotations

import json
import logging
import queue
import signal
import threading
from typing import Any, Callable

import pika

from .config import (
    DOCUMENT_CONTENT_QUEUE,
    RABBITMQ_BLOCKED_CONNECTION_TIMEOUT_SECONDS,
    RABBITMQ_HEARTBEAT_SECONDS,
    RABBITMQ_URL,
    RABBITMQ_WORKER_POLL_SECONDS,
)
from .task_status import set_task_status


LOG = logging.getLogger(__name__)


def prewarm_ocr() -> None:
    LOG.info("prewarming PaddleOCR pipeline")
    from .ocr.processing import pipeline

    LOG.info("PaddleOCR pipeline ready: %s", type(pipeline).__name__)


def process_job(
    job: dict[str, Any],
    process_ocr_fn: Callable[..., dict[str, Any]] | None = None,
    upload_ocr_output_fn: Callable[[dict[str, Any]], dict[str, Any]] | None = None,
    index_ocr_output_fn: Callable[[dict[str, Any]], dict[str, Any]] | None = None,
    set_task_status_fn: Callable[
        [str | None, str | None, str, str | None],
        None,
    ] = set_task_status,
) -> dict[str, Any]:
    task_id = job.get("task_id")
    source = job["source"]
    user_id = job["user_id"]
    document_id = job["document_id"]
    filename = job.get("filename")

    set_task_status_fn(task_id, document_id, "processing", None)

    try:
        if process_ocr_fn is None:
            from .ocr.tasks import process_ocr as process_ocr_fn
        if upload_ocr_output_fn is None:
            from .uploads.tasks import upload_ocr_output as upload_ocr_output_fn
        if index_ocr_output_fn is None:
            from .indexing.tasks import index_ocr_output as index_ocr_output_fn

        ocr_result = process_ocr_fn(source, user_id, document_id, filename)
        upload_result = upload_ocr_output_fn(ocr_result)
        result = index_ocr_output_fn(upload_result)
    except Exception as exc:
        set_task_status_fn(task_id, document_id, "failed", str(exc))
        raise

    if result.get("status") == "indexed":
        set_task_status_fn(task_id, document_id, "successful", None)
    else:
        reason = result.get("reason") or f"Unexpected status: {result.get('status')}"
        set_task_status_fn(task_id, document_id, "failed", reason)

    return result


def rabbitmq_parameters() -> pika.URLParameters:
    params = pika.URLParameters(RABBITMQ_URL)
    if RABBITMQ_HEARTBEAT_SECONDS > 0:
        params.heartbeat = RABBITMQ_HEARTBEAT_SECONDS
    if RABBITMQ_BLOCKED_CONNECTION_TIMEOUT_SECONDS > 0:
        params.blocked_connection_timeout = RABBITMQ_BLOCKED_CONNECTION_TIMEOUT_SECONDS
    return params


def run_with_heartbeat_pump(
    connection: pika.BlockingConnection,
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


def main() -> None:
    logging.basicConfig(level=logging.INFO)
    prewarm_ocr()

    params = rabbitmq_parameters()
    connection = pika.BlockingConnection(params)
    channel = connection.channel()
    channel.queue_declare(queue=DOCUMENT_CONTENT_QUEUE, durable=True)
    channel.basic_qos(prefetch_count=1)

    def stop(signum, frame):
        LOG.info("received signal %s, stopping consumer", signum)
        channel.stop_consuming()

    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)

    def callback(ch, method, properties, body):
        job: dict[str, Any] = {}
        try:
            job = json.loads(body.decode("utf-8"))
            LOG.info(
                "document job received task_id=%s document_id=%s redelivered=%s",
                job.get("task_id"),
                job.get("document_id"),
                method.redelivered,
            )
            result = run_with_heartbeat_pump(connection, lambda: process_job(job))
        except Exception:
            LOG.exception(
                "document job failed task_id=%s document_id=%s",
                job.get("task_id"),
                job.get("document_id"),
            )
            ch.basic_nack(delivery_tag=method.delivery_tag, requeue=False)
            return

        LOG.info(
            "document job complete task_id=%s document_id=%s status=%s",
            job.get("task_id"),
            job.get("document_id"),
            result.get("status"),
        )
        try:
            ch.basic_ack(delivery_tag=method.delivery_tag)
        except Exception:
            LOG.exception(
                "document job ack failed task_id=%s document_id=%s",
                job.get("task_id"),
                job.get("document_id"),
            )
            raise

    channel.basic_consume(queue=DOCUMENT_CONTENT_QUEUE, on_message_callback=callback)
    LOG.info("document-content worker consuming queue=%s", DOCUMENT_CONTENT_QUEUE)
    channel.start_consuming()
    connection.close()


if __name__ == "__main__":
    main()
