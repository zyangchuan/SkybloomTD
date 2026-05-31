import os
from pathlib import Path
from urllib.parse import quote_plus

RABBITMQ_URL = os.getenv("RABBITMQ_URL") or "amqp://guest:guest@rabbitmq:5672/"
DOCUMENT_CONTENT_QUEUE = os.getenv("DOCUMENT_CONTENT_QUEUE", "document.process")
REDIS_URL = os.getenv("REDIS_URL") or "redis://redis:6379/0"
TASK_STATUS_TTL_SECONDS = int(os.getenv("TASK_STATUS_TTL_SECONDS", "604800"))

INPUT_ROOT = Path(os.getenv("INPUT_ROOT", "/temp"))
OUTPUT_ROOT = Path("/output")

AWS_S3_BUCKET = os.getenv("AWS_S3_BUCKET")
AWS_S3_PREFIX = os.getenv("AWS_S3_PREFIX", "").strip("/")
AWS_REGION = os.getenv("AWS_REGION") or os.getenv("AWS_DEFAULT_REGION")
AWS_S3_ENDPOINT_URL = os.getenv("AWS_S3_ENDPOINT_URL")

POSTGRES_HOST = os.getenv("POSTGRES_HOST") or os.getenv("AWS_RDS_POSTGRES_HOST")
POSTGRES_PORT = os.getenv("POSTGRES_PORT", "5432")
POSTGRES_DB = os.getenv("POSTGRES_DB")
POSTGRES_USER = os.getenv("POSTGRES_USER")
POSTGRES_PASSWORD = os.getenv("POSTGRES_PASSWORD")
POSTGRES_SSLMODE = os.getenv("POSTGRES_SSLMODE", "require")


def normalize_database_url(url: str | None) -> str | None:
    if not url:
        return None
    if url.startswith("postgres://"):
        return url.replace("postgres://", "postgresql+psycopg://", 1)
    if url.startswith("postgresql://"):
        return url.replace("postgresql://", "postgresql+psycopg://", 1)
    return url


def database_url_from_parts() -> str | None:
    if not all([POSTGRES_HOST, POSTGRES_DB, POSTGRES_USER, POSTGRES_PASSWORD]):
        return None

    username = quote_plus(POSTGRES_USER)
    password = quote_plus(POSTGRES_PASSWORD)
    database = quote_plus(POSTGRES_DB)
    url = (
        f"postgresql+psycopg://{username}:{password}"
        f"@{POSTGRES_HOST}:{POSTGRES_PORT}/{database}"
    )
    if POSTGRES_SSLMODE:
        url = f"{url}?sslmode={quote_plus(POSTGRES_SSLMODE)}"
    return url


DATABASE_URL = normalize_database_url(
    os.getenv("DATABASE_URL") or os.getenv("POSTGRES_DSN")
) or database_url_from_parts()
