import json
import logging
import signal
from typing import Any

import pika

from .config import DOCUMENT_CONTENT_QUEUE, RABBITMQ_URL
from .indexing.tasks import index_ocr_output
from .ocr.tasks import process_ocr
from .uploads.tasks import upload_ocr_output


LOG = logging.getLogger(__name__)


def process_job(job: dict[str, Any]) -> dict[str, Any]:
    source = job["source"]
    user_id = job["user_id"]
    document_id = job["document_id"]
    filename = job.get("filename")

    ocr_result = process_ocr(source, user_id, document_id, filename)
    upload_result = upload_ocr_output(ocr_result)
    return index_ocr_output(upload_result)


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
        try:
            job = json.loads(body.decode("utf-8"))
            result = process_job(job)
            LOG.info(
                "document job complete task_id=%s document_id=%s status=%s",
                job.get("task_id"),
                job.get("document_id"),
                result.get("status"),
            )
            ch.basic_ack(delivery_tag=method.delivery_tag)
        except Exception:
            LOG.exception("document job failed")
            ch.basic_nack(delivery_tag=method.delivery_tag, requeue=False)

    channel.basic_consume(queue=DOCUMENT_CONTENT_QUEUE, on_message_callback=callback)
    LOG.info("document-content worker consuming queue=%s", DOCUMENT_CONTENT_QUEUE)
    channel.start_consuming()
    connection.close()


if __name__ == "__main__":
    main()
