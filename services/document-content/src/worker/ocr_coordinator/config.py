import os


def required_env(key: str) -> str:
    value = os.getenv(key)
    if value is None or value == "":
        raise RuntimeError(f"Missing required environment variable: {key}")
    return value


# RabbitMQ configuration
RABBITMQ_URL = "amqp://guest:guest@rabbitmq:5672/"
RABBITMQ_HEARTBEAT_SECONDS = 600
RABBITMQ_BLOCKED_CONNECTION_TIMEOUT_SECONDS = 300
RABBITMQ_WORKER_POLL_SECONDS = 10
DOCUMENT_CONTENT_OCR_QUEUE = "document-content-ocr-queue"
DOCUMENT_CONTENT_INDEXING_QUEUE = "document-content-indexing-queue"

# Redis configuration
REDIS_URL = "redis://redis:6379/0"
TASK_STATUS_TTL_SECONDS = 604800

# RunPod configuration
RUNPOD_API_KEY = required_env("RUNPOD_API_KEY")
RUNPOD_ENDPOINT_ID = required_env("RUNPOD_ENDPOINT_ID")
RUNPOD_POLL_INTERVAL_SECONDS = 5
RUNPOD_JOB_TIMEOUT_SECONDS = 1800
OCR_PRESIGNED_URL_TTL_SECONDS = 3600

# S3-compatible storage configuration
AWS_REGION = required_env("AWS_REGION")
AWS_S3_ENDPOINT_URL = required_env("AWS_S3_ENDPOINT_URL")
