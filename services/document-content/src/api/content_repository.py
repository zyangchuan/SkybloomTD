import hashlib
import uuid
from datetime import datetime
from functools import lru_cache
from typing import Any

import boto3
import redis
from redis.exceptions import RedisError
from sqlalchemy import DateTime, Integer, Text, create_engine, select
from sqlalchemy.dialects.postgresql import UUID
from sqlalchemy.orm import DeclarativeBase, Mapped, Session, mapped_column
from sqlalchemy.sql import func

from config import (
    AWS_REGION,
    AWS_S3_ENDPOINT_URL,
    DATABASE_URL,
    MARKDOWN_CACHE_TTL_SECONDS,
    REDIS_URL,
)


class ContentRequestError(ValueError):
    pass


class ContentNotFoundError(ValueError):
    pass


class ContentUnavailableError(ValueError):
    pass


class Base(DeclarativeBase):
    pass


class Document(Base):
    __tablename__ = "documents"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True)
    user_id: Mapped[uuid.UUID | None] = mapped_column(UUID(as_uuid=True))
    s3_bucket: Mapped[str | None] = mapped_column(Text)
    s3_key: Mapped[str | None] = mapped_column(Text)
    filename: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(
        DateTime,
        server_default=func.current_timestamp(),
    )


class SubChapter(Base):
    __tablename__ = "sub_chapters"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True)
    document_id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True))
    chapter_id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True))
    sub_chapter_index: Mapped[int | None] = mapped_column(Integer)
    title: Mapped[str | None] = mapped_column(Text)
    start_line: Mapped[int | None] = mapped_column(Integer)
    end_line: Mapped[int | None] = mapped_column(Integer)


def parse_uuid(value: str, field_name: str) -> uuid.UUID:
    try:
        return uuid.UUID(str(value))
    except ValueError as exc:
        raise ContentRequestError(f"{field_name} must be a valid UUID") from exc


def indexed_user_uuid(value: str) -> uuid.UUID:
    try:
        return uuid.UUID(str(value))
    except ValueError:
        return uuid.uuid5(uuid.NAMESPACE_URL, f"ocr:user:{value}")


@lru_cache(maxsize=1)
def engine():
    if not DATABASE_URL:
        raise RuntimeError("DATABASE_URL or POSTGRES_DSN must be configured")
    return create_engine(DATABASE_URL, pool_pre_ping=True)


@lru_cache(maxsize=1)
def redis_client() -> redis.Redis:
    return redis.Redis.from_url(REDIS_URL, decode_responses=True)


@lru_cache(maxsize=1)
def s3_client():
    kwargs = {}
    if AWS_REGION:
        kwargs["region_name"] = AWS_REGION
    if AWS_S3_ENDPOINT_URL:
        kwargs["endpoint_url"] = AWS_S3_ENDPOINT_URL
    return boto3.client("s3", **kwargs)


def markdown_cache_key(document_id: str, s3_bucket: str, s3_key: str) -> str:
    source_hash = hashlib.sha256(f"{s3_bucket}/{s3_key}".encode("utf-8")).hexdigest()
    return f"document_markdown:{document_id}:{source_hash}"


def download_markdown_from_s3(s3_bucket: str, s3_key: str) -> str:
    response = s3_client().get_object(Bucket=s3_bucket, Key=s3_key)
    return response["Body"].read().decode("utf-8")


def fetch_markdown(
    *,
    document_id: str,
    s3_bucket: str,
    s3_key: str,
) -> tuple[str, dict[str, Any]]:
    cache_key = markdown_cache_key(document_id, s3_bucket, s3_key)

    try:
        cached = redis_client().get(cache_key)
    except RedisError:
        cached = None

    if cached is not None:
        return cached, {
            "markdown_cache_hit": True,
            "markdown_cache_key": cache_key,
        }

    markdown = download_markdown_from_s3(s3_bucket, s3_key)

    try:
        redis_client().setex(cache_key, MARKDOWN_CACHE_TTL_SECONDS, markdown)
    except RedisError:
        pass

    return markdown, {
        "markdown_cache_hit": False,
        "markdown_cache_key": cache_key,
    }


def slice_markdown_lines(
    markdown: str,
    start_line: int | None,
    end_line: int | None,
) -> str:
    if start_line is None or end_line is None:
        raise ContentUnavailableError("Sub-chapter line range is missing")
    if start_line < 1:
        raise ContentUnavailableError("Sub-chapter start_line must be at least 1")
    if end_line < start_line:
        raise ContentUnavailableError("Sub-chapter end_line must be after start_line")

    lines = markdown.splitlines()
    if start_line > len(lines):
        raise ContentUnavailableError(
            "Sub-chapter start_line is outside the markdown document"
        )

    selected_lines = lines[start_line - 1 : min(end_line, len(lines))]
    source_text = "\n".join(selected_lines).strip()
    if not source_text:
        raise ContentUnavailableError("No markdown text found for the sub_chapter_id")
    return source_text


def fetch_sub_chapter_content(
    *,
    user_id: str,
    sub_chapter_id: str,
    max_chars: int = 0,
) -> dict[str, Any]:
    if not user_id:
        raise ContentRequestError("user_id is required")
    if max_chars < 0:
        raise ContentRequestError("max_chars must be zero or greater")

    sub_chapter_uuid = parse_uuid(sub_chapter_id, "sub_chapter_id")
    user_uuid = indexed_user_uuid(user_id)

    with Session(engine()) as session:
        row = session.execute(
            select(SubChapter, Document)
            .join(Document, SubChapter.document_id == Document.id)
            .where(
                SubChapter.id == sub_chapter_uuid,
                Document.user_id == user_uuid,
            )
        ).one_or_none()

        if row is None:
            raise ContentNotFoundError(
                "No sub_chapter found for the provided user_id and sub_chapter_id"
            )

        sub_chapter, document = row
        content = {
            "normalized_user_id": str(user_uuid),
            "requested_user_id": user_id,
            "sub_chapter_id": str(sub_chapter.id),
            "document_id": str(sub_chapter.document_id),
            "chapter_id": str(sub_chapter.chapter_id),
            "sub_chapter_index": sub_chapter.sub_chapter_index or 0,
            "title": sub_chapter.title or "",
            "start_line": sub_chapter.start_line or 0,
            "end_line": sub_chapter.end_line or 0,
            "s3_bucket": document.s3_bucket,
            "s3_key": document.s3_key,
        }

    if not content["s3_bucket"] or not content["s3_key"]:
        raise ContentUnavailableError("Document markdown S3 location is missing")

    markdown, cache_metadata = fetch_markdown(
        document_id=content["document_id"],
        s3_bucket=content["s3_bucket"],
        s3_key=content["s3_key"],
    )
    source_text = slice_markdown_lines(
        markdown,
        content["start_line"],
        content["end_line"],
    )

    source_char_count = len(source_text)
    source_truncated = bool(max_chars and source_char_count > max_chars)
    if source_truncated:
        source_text = source_text[:max_chars]

    return {
        **content,
        "source_text": source_text,
        "source_chunk_ids": [],
        "chunk_count": 0,
        "candidate_chunk_count": 0,
        "chunk_lookup_strategy": "s3_markdown_line_range",
        "source_char_count": source_char_count,
        "source_truncated": source_truncated,
        "source_content_hash": hashlib.sha256(source_text.encode("utf-8")).hexdigest(),
        **cache_metadata,
    }
