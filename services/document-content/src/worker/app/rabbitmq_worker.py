import json
import logging
import signal
from typing import Any, Callable

import pika

from .config import DOCUMENT_CONTENT_QUEUE, RABBITMQ_URL
from .task_status import set_task_status


LOG = logging.getLogger(__name__)


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
    if process_ocr_fn is None:
        from .ocr.tasks import process_ocr as process_ocr_fn
    if upload_ocr_output_fn is None:
        from .uploads.tasks import upload_ocr_output as upload_ocr_output_fn
    if index_ocr_output_fn is None:
        from .indexing.tasks import index_ocr_output as index_ocr_output_fn

    task_id = job.get("task_id")
    source = job["source"]
    user_id = job["user_id"]
    document_id = job["document_id"]
    filename = job.get("filename")

    set_task_status_fn(task_id, document_id, "processing", None)

    try:
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


def main() -> None:
    logging.basicConfig(level=logging.INFO)
    params = pika.URLParameters(RABBITMQ_URL)
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
            result = process_job(job)
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
