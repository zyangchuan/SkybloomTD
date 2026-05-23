from celery import Celery

from .config import REDIS_URL

celery_app = Celery(
    "document_content_worker",
    broker=REDIS_URL,
    backend=REDIS_URL,
    include=[
        "app.ocr.tasks",
        "app.uploads.tasks",
        "app.indexing.tasks",
    ],
)
