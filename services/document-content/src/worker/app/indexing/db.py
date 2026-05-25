import uuid
from dataclasses import dataclass
from datetime import datetime
from typing import Any

from pgvector.sqlalchemy import Vector
from sqlalchemy import Boolean, DateTime, ForeignKey, Integer, Text, create_engine, delete
from sqlalchemy import text as sql_text
from sqlalchemy.dialects.postgresql import JSONB, UUID
from sqlalchemy.orm import DeclarativeBase, Mapped, Session, mapped_column
from sqlalchemy.sql import func

from ..config import DATABASE_URL, EMBEDDING_DIMENSIONS
from .sections import ChapterSection, Chunk, SubChapterSection


class Base(DeclarativeBase):
    pass


class Document(Base):
    __tablename__ = "documents"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True)
    user_id: Mapped[uuid.UUID | None] = mapped_column(UUID(as_uuid=True))
    s3_bucket: Mapped[str | None] = mapped_column(Text)
    s3_key: Mapped[str | None] = mapped_column(Text)
    filename: Mapped[str | None] = mapped_column(Text)
    task_id: Mapped[str | None] = mapped_column(Text)
    is_ready: Mapped[bool] = mapped_column(
        Boolean,
        server_default=sql_text("false"),
        default=False,
    )
    source_type: Mapped[str | None] = mapped_column(Text)
    source_bucket: Mapped[str | None] = mapped_column(Text)
    source_key: Mapped[str | None] = mapped_column(Text)
    source_path: Mapped[str | None] = mapped_column(Text)
    source_content_type: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(
        DateTime,
        server_default=func.current_timestamp(),
    )


class Chapter(Base):
    __tablename__ = "chapters"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True)
    document_id: Mapped[uuid.UUID] = mapped_column(
        UUID(as_uuid=True),
        ForeignKey("documents.id", ondelete="CASCADE"),
        index=True,
    )
    chapter_index: Mapped[int | None] = mapped_column(Integer)
    title: Mapped[str | None] = mapped_column(Text)
    start_line: Mapped[int | None] = mapped_column(Integer)
    end_line: Mapped[int | None] = mapped_column(Integer)
    created_at: Mapped[datetime] = mapped_column(
        DateTime,
        server_default=func.current_timestamp(),
    )


class SubChapter(Base):
    __tablename__ = "sub_chapters"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True)
    document_id: Mapped[uuid.UUID] = mapped_column(
        UUID(as_uuid=True),
        ForeignKey("documents.id", ondelete="CASCADE"),
        index=True,
    )
    chapter_id: Mapped[uuid.UUID] = mapped_column(
        UUID(as_uuid=True),
        ForeignKey("chapters.id", ondelete="CASCADE"),
    )
    sub_chapter_index: Mapped[int | None] = mapped_column(Integer)
    title: Mapped[str | None] = mapped_column(Text)
    start_line: Mapped[int | None] = mapped_column(Integer)
    end_line: Mapped[int | None] = mapped_column(Integer)
    created_at: Mapped[datetime] = mapped_column(
        DateTime,
        server_default=func.current_timestamp(),
    )


class ChunkRecord(Base):
    __tablename__ = "chunks"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True)
    document_id: Mapped[uuid.UUID] = mapped_column(
        UUID(as_uuid=True),
        ForeignKey("documents.id", ondelete="CASCADE"),
        index=True,
    )
    chapter_id: Mapped[uuid.UUID] = mapped_column(
        UUID(as_uuid=True),
        ForeignKey("chapters.id", ondelete="CASCADE"),
        index=True,
    )
    sub_chapter_id: Mapped[uuid.UUID | None] = mapped_column(
        UUID(as_uuid=True),
        ForeignKey(
            "sub_chapters.id",
            name="chunks_sub_chapter_id_fkey",
            ondelete="SET NULL",
        ),
        index=True,
    )
    chunk_index: Mapped[int | None] = mapped_column(Integer)
    content: Mapped[str | None] = mapped_column(Text)
    embedding: Mapped[list[float] | None] = mapped_column(
        Vector(int(EMBEDDING_DIMENSIONS))
    )
    metadata_json: Mapped[dict[str, Any] | None] = mapped_column("metadata", JSONB)
    created_at: Mapped[datetime] = mapped_column(
        DateTime,
        server_default=func.current_timestamp(),
    )


@dataclass(frozen=True)
class DocumentIndexRecord:
    document_id: str
    user_id: str
    s3_bucket: str
    s3_key: str
    filename: str
    chapters: list[ChapterSection]
    sub_chapters: list[SubChapterSection]
    chunks: list[Chunk]
    embeddings: list[list[float] | None]


def db_uuid(value: str, namespace: str) -> uuid.UUID:
    try:
        return uuid.UUID(str(value))
    except ValueError:
        return uuid.uuid5(uuid.NAMESPACE_URL, f"ocr:{namespace}:{value}")


def engine():
    if not DATABASE_URL:
        raise RuntimeError("DATABASE_URL or POSTGRES_DSN must be configured")
    return create_engine(DATABASE_URL, pool_pre_ping=True)


def ensure_schema() -> None:
    db_engine = engine()

    with db_engine.begin() as conn:
        conn.execute(sql_text("CREATE EXTENSION IF NOT EXISTS vector"))

    Base.metadata.create_all(db_engine)

    statements = [
        "ALTER TABLE documents ADD COLUMN IF NOT EXISTS task_id TEXT",
        "ALTER TABLE documents ADD COLUMN IF NOT EXISTS is_ready BOOLEAN DEFAULT false",
        "UPDATE documents SET is_ready = false WHERE is_ready IS NULL",
        "ALTER TABLE documents ALTER COLUMN is_ready SET DEFAULT false",
        "ALTER TABLE documents ALTER COLUMN is_ready SET NOT NULL",
        "ALTER TABLE documents ADD COLUMN IF NOT EXISTS source_type TEXT",
        "ALTER TABLE documents ADD COLUMN IF NOT EXISTS source_bucket TEXT",
        "ALTER TABLE documents ADD COLUMN IF NOT EXISTS source_key TEXT",
        "ALTER TABLE documents ADD COLUMN IF NOT EXISTS source_path TEXT",
        "ALTER TABLE documents ADD COLUMN IF NOT EXISTS source_content_type TEXT",
        "CREATE INDEX IF NOT EXISTS documents_task_id_idx ON documents(task_id)",
        "CREATE INDEX IF NOT EXISTS documents_user_id_idx ON documents(user_id)",
        "ALTER TABLE chunks ADD COLUMN IF NOT EXISTS sub_chapter_id UUID",
        "ALTER TABLE chunks ADD COLUMN IF NOT EXISTS content TEXT",
        """
        DO $$
        BEGIN
            IF NOT EXISTS (
                SELECT 1
                FROM pg_constraint
                WHERE conname = 'chunks_sub_chapter_id_fkey'
            ) THEN
                ALTER TABLE chunks
                ADD CONSTRAINT chunks_sub_chapter_id_fkey
                FOREIGN KEY (sub_chapter_id)
                REFERENCES sub_chapters(id)
                ON DELETE SET NULL;
            END IF;
        END
        $$
        """,
        "CREATE INDEX IF NOT EXISTS chunks_document_chapter_idx ON chunks(document_id, chapter_id)",
        "CREATE INDEX IF NOT EXISTS chunks_sub_chapter_idx ON chunks(sub_chapter_id)",
        """
        CREATE INDEX IF NOT EXISTS chunks_embedding_hnsw_idx
        ON chunks USING hnsw (embedding vector_cosine_ops)
        """,
    ]

    with db_engine.begin() as conn:
        for statement in statements:
            conn.execute(sql_text(statement))


def sub_chapter_for_chunk(
    chunk: Chunk,
    sub_chapters: list[SubChapterSection],
) -> SubChapterSection | None:
    return next(
        (
            item
            for item in sub_chapters
            if item.chapter_index == chunk.chapter_index
            and item.start_line <= chunk.start_line <= item.end_line
        ),
        None,
    )


def insert_document_index(record: DocumentIndexRecord) -> dict[str, Any]:
    if len(record.chunks) != len(record.embeddings):
        raise ValueError("Chunk count does not match embedding count")

    document_id = db_uuid(record.document_id, "document")
    user_id = db_uuid(record.user_id, "user")
    chapter_ids: dict[int, uuid.UUID] = {}
    sub_chapter_ids: dict[tuple[int, int], uuid.UUID] = {}
    chapter_lookup = {chapter.index: chapter for chapter in record.chapters}

    ensure_schema()
    db_engine = engine()

    with Session(db_engine) as session:
        with session.begin():
            session.execute(delete(ChunkRecord).where(ChunkRecord.document_id == document_id))
            session.execute(delete(SubChapter).where(SubChapter.document_id == document_id))
            session.execute(delete(Chapter).where(Chapter.document_id == document_id))

            document = session.get(Document, document_id)
            if document is None:
                document = Document(id=document_id)
                session.add(document)
            document.user_id = user_id
            document.s3_bucket = record.s3_bucket
            document.s3_key = record.s3_key
            document.filename = record.filename
            document.is_ready = True
            session.flush()

            for chapter in record.chapters:
                chapter_id = uuid.uuid4()
                chapter_ids[chapter.index] = chapter_id
                session.add(
                    Chapter(
                        id=chapter_id,
                        document_id=document_id,
                        chapter_index=chapter.index,
                        title=chapter.title,
                        start_line=chapter.start_line,
                        end_line=chapter.end_line,
                    )
                )
            session.flush()

            for sub_chapter in record.sub_chapters:
                sub_chapter_id = uuid.uuid4()
                sub_chapter_ids[(sub_chapter.chapter_index, sub_chapter.index)] = (
                    sub_chapter_id
                )
                session.add(
                    SubChapter(
                        id=sub_chapter_id,
                        document_id=document_id,
                        chapter_id=chapter_ids[sub_chapter.chapter_index],
                        sub_chapter_index=sub_chapter.index,
                        title=sub_chapter.title,
                        start_line=sub_chapter.start_line,
                        end_line=sub_chapter.end_line,
                    )
                )
            session.flush()

            for chunk, embedding in zip(record.chunks, record.embeddings, strict=True):
                chapter = chapter_lookup[chunk.chapter_index]
                sub_chapter = sub_chapter_for_chunk(chunk, record.sub_chapters)
                sub_chapter_id = None
                if sub_chapter is not None:
                    sub_chapter_id = sub_chapter_ids[
                        (sub_chapter.chapter_index, sub_chapter.index)
                    ]

                metadata = {
                    "chapter_index": chapter.index,
                    "chapter_title": chapter.title,
                    "start_line": chunk.start_line,
                    "end_line": chunk.end_line,
                    "s3_bucket": record.s3_bucket,
                    "s3_key": record.s3_key,
                    "filename": record.filename,
                }
                if sub_chapter is not None:
                    metadata.update(
                        {
                            "sub_chapter_index": sub_chapter.index,
                            "sub_chapter_title": sub_chapter.title,
                        }
                    )

                session.add(
                    ChunkRecord(
                        id=uuid.uuid4(),
                        document_id=document_id,
                        chapter_id=chapter_ids[chunk.chapter_index],
                        sub_chapter_id=sub_chapter_id,
                        chunk_index=chunk.chunk_index,
                        content=None,
                        embedding=embedding,
                        metadata_json=metadata,
                    )
                )

    return {
        "document_id": str(document_id),
        "chapter_count": len(record.chapters),
        "sub_chapter_count": len(record.sub_chapters),
        "chunk_count": len(record.chunks),
    }
