from __future__ import annotations

import json
import logging
import signal
from pathlib import Path
from typing import Any, Callable

from indexing_worker.config import DOCUMENT_CONTENT_INDEXING_QUEUE, OUTPUT_ROOT
from indexing_worker.rabbitmq import rabbitmq_parameters, run_with_heartbeat_pump
from indexing_worker.task_status import set_task_status

LOG = logging.getLogger(__name__)


def process_indexing_job(
    job: dict[str, Any],
    index_ocr_output_fn: Callable[[dict[str, Any]], dict[str, Any]] | None = None,
    set_task_status_fn: Callable[[str | None, str | None, str, str | None], None] = set_task_status,
) -> dict[str, Any]:
    task_id = job.get("task_id")
    document_id = job.get("document_id")

    try:
        if index_ocr_output_fn is None:
            from .indexing.tasks import index_ocr_output as index_ocr_output_fn

        upload_result = prepare_upload_result(job)
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


def prepare_upload_result(
    job: dict[str, Any],
    download_file_from_s3_fn: Callable[[str, str, Path], Path] | None = None,
) -> dict[str, Any]:
    if download_file_from_s3_fn is None:
        from .indexing.storage import download_file_from_s3 as download_file_from_s3_fn

    output = job["ocr_output"]
    user_id = job["user_id"]
    document_id = job["document_id"]
    local_markdown_path = Path(OUTPUT_ROOT) / user_id / document_id / "output.md"
    download_file_from_s3_fn(
        output["s3_bucket"],
        output["markdown_key"],
        local_markdown_path,
    )
    return {
        "status": "uploaded",
        "user_id": user_id,
        "document_id": document_id,
        "source_filename": job["source_filename"],
        "local_markdown_path": str(local_markdown_path),
        "s3_bucket": output["s3_bucket"],
        "s3_markdown_key": output["markdown_key"],
    }


def main() -> None:
    logging.basicConfig(level=logging.INFO)

    import pika

    params = rabbitmq_parameters()
    connection = pika.BlockingConnection(params)
    channel = connection.channel()
    channel.queue_declare(queue=DOCUMENT_CONTENT_INDEXING_QUEUE, durable=True)
    channel.basic_qos(prefetch_count=1)

    def stop(signum, frame):
        LOG.info("received signal %s, stopping indexing consumer", signum)
        channel.stop_consuming()

    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)

    def callback(ch, method, properties, body):
        job: dict[str, Any] = {}
        try:
            job = json.loads(body.decode("utf-8"))
            LOG.info(
                "document indexing job received task_id=%s document_id=%s redelivered=%s",
                job.get("task_id"),
                job.get("document_id"),
                method.redelivered,
            )
            result = run_with_heartbeat_pump(
                connection,
                lambda: process_indexing_job(job),
            )
        except Exception:
            LOG.exception(
                "document indexing job failed task_id=%s document_id=%s",
                job.get("task_id"),
                job.get("document_id"),
            )
            ch.basic_nack(delivery_tag=method.delivery_tag, requeue=False)
            return

        LOG.info(
            "document indexing job complete task_id=%s document_id=%s status=%s",
            job.get("task_id"),
            job.get("document_id"),
            result.get("status"),
        )
        ch.basic_ack(delivery_tag=method.delivery_tag)

    channel.basic_consume(
        queue=DOCUMENT_CONTENT_INDEXING_QUEUE,
        on_message_callback=callback,
    )
    LOG.info(
        "document-content indexing worker consuming queue=%s",
        DOCUMENT_CONTENT_INDEXING_QUEUE,
    )
    channel.start_consuming()
    connection.close()


if __name__ == "__main__":
    main()
