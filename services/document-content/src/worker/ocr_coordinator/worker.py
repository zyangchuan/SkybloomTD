from __future__ import annotations

import json
import logging
import signal
from typing import Any, Callable

from ocr_coordinator.config import (
    DOCUMENT_CONTENT_INDEXING_QUEUE,
    DOCUMENT_CONTENT_OCR_QUEUE,
    OCR_PRESIGNED_URL_TTL_SECONDS,
)
from ocr_coordinator.rabbitmq import rabbitmq_parameters, run_with_heartbeat_pump
from ocr_coordinator.storage import (
    output_markdown_s3_key,
    presigned_download_url,
    presigned_upload_url,
    source_s3_key,
)
from ocr_coordinator.task_status import set_task_status

LOG = logging.getLogger(__name__)


def process_job(
    job: dict[str, Any],
    runpod_ocr_fn: Callable[[dict[str, Any]], dict[str, Any]] | None = None,
    set_task_status_fn: Callable[[str | None, str | None, str, str | None], None] = set_task_status,
) -> dict[str, Any]:
    task_id = job.get("task_id")
    document_id = job["document_id"]

    set_task_status_fn(task_id, document_id, "processing", None)

    try:
        if runpod_ocr_fn is None:
            runpod_ocr_fn = process_runpod_ocr
        runpod_result = runpod_ocr_fn(job)
        indexing_job = build_indexing_job(job, runpod_result)
    except Exception as exc:
        set_task_status_fn(task_id, document_id, "failed", str(exc))
        raise

    return {
        "status": "ocr_completed",
        "task_id": task_id,
        "document_id": document_id,
        "indexing_job": indexing_job,
    }


def process_runpod_ocr(job: dict[str, Any]) -> dict[str, Any]:
    from ocr_coordinator.runpod_client import submit_ocr_job

    source = job["source"]
    user_id = job["user_id"]
    document_id = job["document_id"]
    source_filename = source["source_filename"]
    bucket = source["s3_bucket"]
    source_key = source_s3_key(user_id, document_id, source_filename)
    markdown_key = output_markdown_s3_key(user_id, document_id)

    output = submit_ocr_job(
        {
            "job_id": job["task_id"],
            "source_url": presigned_download_url(
                bucket,
                source_key,
                OCR_PRESIGNED_URL_TTL_SECONDS,
            ),
            "markdown_put_url": presigned_upload_url(
                bucket,
                markdown_key,
                OCR_PRESIGNED_URL_TTL_SECONDS,
            ),
        }
    )
    if output.get("status") != "ocr_completed":
        raise RuntimeError(f"RunPod OCR did not complete successfully: {output}")

    return {
        "status": "ocr_completed",
        "user_id": user_id,
        "document_id": document_id,
        "source_filename": source_filename,
        "s3_bucket": bucket,
        "s3_markdown_key": markdown_key,
        "runpod": output,
    }


def build_indexing_job(job: dict[str, Any], ocr_result: dict[str, Any]) -> dict[str, Any]:
    if ocr_result.get("status") != "ocr_completed":
        raise RuntimeError(f"OCR did not produce an indexable artifact: {ocr_result}")

    return {
        "job_type": "document.index",
        "task_id": job.get("task_id"),
        "user_id": job["user_id"],
        "document_id": job["document_id"],
        "source_filename": ocr_result["source_filename"],
        "ocr_output": {
            "s3_bucket": ocr_result["s3_bucket"],
            "markdown_key": ocr_result["s3_markdown_key"],
        },
    }


def publish_json(
    channel,
    queue_name: str,
    message_id: str | None,
    value: dict[str, Any],
) -> None:
    import pika

    channel.queue_declare(queue=queue_name, durable=True)
    channel.basic_publish(
        exchange="",
        routing_key=queue_name,
        body=json.dumps(value).encode("utf-8"),
        properties=pika.BasicProperties(
            content_type="application/json",
            delivery_mode=2,
            message_id=message_id,
        ),
    )


def main() -> None:
    logging.basicConfig(level=logging.INFO)

    import pika

    params = rabbitmq_parameters()
    connection = pika.BlockingConnection(params)
    channel = connection.channel()
    channel.queue_declare(queue=DOCUMENT_CONTENT_OCR_QUEUE, durable=True)
    channel.queue_declare(queue=DOCUMENT_CONTENT_INDEXING_QUEUE, durable=True)
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
                "document OCR job received task_id=%s document_id=%s redelivered=%s",
                job.get("task_id"),
                job.get("document_id"),
                method.redelivered,
            )
            result = run_with_heartbeat_pump(connection, lambda: process_job(job))
            publish_json(
                ch,
                DOCUMENT_CONTENT_INDEXING_QUEUE,
                job.get("task_id"),
                result["indexing_job"],
            )
        except Exception:
            LOG.exception(
                "document OCR job failed task_id=%s document_id=%s",
                job.get("task_id"),
                job.get("document_id"),
            )
            ch.basic_nack(delivery_tag=method.delivery_tag, requeue=False)
            return

        LOG.info(
            "document OCR job complete task_id=%s document_id=%s",
            job.get("task_id"),
            job.get("document_id"),
        )
        ch.basic_ack(delivery_tag=method.delivery_tag)

    channel.basic_consume(
        queue=DOCUMENT_CONTENT_OCR_QUEUE,
        on_message_callback=callback,
    )
    LOG.info("document-content OCR coordinator consuming queue=%s", DOCUMENT_CONTENT_OCR_QUEUE)
    channel.start_consuming()
    connection.close()


if __name__ == "__main__":
    main()
