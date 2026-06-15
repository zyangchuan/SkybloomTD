import os
from pathlib import Path
from urllib.parse import quote_plus

# RabbitMQ configuration
RABBITMQ_URL = os.getenv("RABBITMQ_URL")
RABBITMQ_HEARTBEAT_SECONDS = 600
RABBITMQ_BLOCKED_CONNECTION_TIMEOUT_SECONDS = 300
RABBITMQ_WORKER_POLL_SECONDS = 10
DOCUMENT_CONTENT_QUEUE = "document-content-queue"

# Redis configuration
REDIS_URL = os.getenv("REDIS_URL")
TASK_STATUS_TTL_SECONDS = os.getenv("TASK_STATUS_TTL_SECONDS")

# File storage configuration
OUTPUT_ROOT = Path("/output")
AWS_S3_BUCKET = os.getenv("AWS_S3_BUCKET")
AWS_REGION = os.getenv("AWS_REGION")
AWS_S3_ENDPOINT_URL = os.getenv("AWS_S3_ENDPOINT_URL")

# Database configuration
POSTGRES_HOST = os.getenv("POSTGRES_HOST")
POSTGRES_PORT = os.getenv("POSTGRES_PORT")
POSTGRES_DB = os.getenv("POSTGRES_DB")
POSTGRES_USER = os.getenv("POSTGRES_USER")
POSTGRES_PASSWORD = os.getenv("POSTGRES_PASSWORD")
POSTGRES_SSLMODE = os.getenv("POSTGRES_SSLMODE")

def database_url_from_parts() -> str | None:
    if not all([POSTGRES_HOST, POSTGRES_PORT, POSTGRES_DB, POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_SSLMODE]):
        return None

    username = quote_plus(POSTGRES_USER)
    password = quote_plus(POSTGRES_PASSWORD)
    database = quote_plus(POSTGRES_DB)
    sslmode = quote_plus(POSTGRES_SSLMODE)
    url = (
        f"postgresql+psycopg://{username}:{password}"
        f"@{POSTGRES_HOST}:{POSTGRES_PORT}/{database}"
        f"?sslmode={sslmode}"
    )

    return url

DATABASE_URL = database_url_from_parts()

# LLM configuration for sectioning
SECTIONING_LLM_MODEL = os.getenv("SECTIONING_LLM_MODEL")
SECTIONING_LLM_TIMEOUT_SECONDS = 120
SECTIONING_LLM_MAX_RETRIES = 2
SECTIONING_OUTLINE_MAX_REPAIRS = 2
