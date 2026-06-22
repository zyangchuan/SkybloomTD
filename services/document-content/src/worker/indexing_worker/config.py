import os
from pathlib import Path
from urllib.parse import quote_plus


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
DOCUMENT_CONTENT_INDEXING_QUEUE = "document-content-indexing-queue"

# Redis configuration
REDIS_URL = "redis://redis:6379/0"
TASK_STATUS_TTL_SECONDS = 604800

# File storage configuration
INPUT_ROOT = Path("/temp")
OUTPUT_ROOT = Path("/output")
AWS_REGION = required_env("AWS_REGION")
AWS_S3_ENDPOINT_URL = required_env("AWS_S3_ENDPOINT_URL")

# Database configuration
POSTGRES_HOST = required_env("POSTGRES_HOST")
POSTGRES_PORT = required_env("POSTGRES_PORT")
POSTGRES_DB = required_env("POSTGRES_DB")
POSTGRES_USER = required_env("POSTGRES_USER")
POSTGRES_PASSWORD = required_env("POSTGRES_PASSWORD")
POSTGRES_SSLMODE = required_env("POSTGRES_SSLMODE")


def database_url_from_parts() -> str:
    username = quote_plus(POSTGRES_USER)
    password = quote_plus(POSTGRES_PASSWORD)
    database = quote_plus(POSTGRES_DB)
    sslmode = quote_plus(POSTGRES_SSLMODE)
    return (
        f"postgresql+psycopg://{username}:{password}"
        f"@{POSTGRES_HOST}:{POSTGRES_PORT}/{database}"
        f"?sslmode={sslmode}"
    )


DATABASE_URL = database_url_from_parts()

# LLM configuration for sectioning
SECTIONING_LLM_MODEL = required_env("SECTIONING_LLM_MODEL")
SECTIONING_LLM_TIMEOUT_SECONDS = 120
SECTIONING_LLM_MAX_RETRIES = 2
SECTIONING_OUTLINE_MAX_REPAIRS = 2
