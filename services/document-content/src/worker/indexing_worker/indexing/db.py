import uuid
from dataclasses import dataclass
from datetime import datetime

from sqlalchemy import Boolean, DateTime, ForeignKey, Integer, Text, create_engine, delete
from sqlalchemy import text as sql_text
from sqlalchemy.dialects.postgresql import UUID
from sqlalchemy.orm import DeclarativeBase, Mapped, Session, mapped_column
from sqlalchemy.sql import func

from indexing_worker.config import DATABASE_URL
from .types import ChapterSection, SubChapterSection


class Base(DeclarativeBase):
    pass


class Document(Base):
    __tablename__ = "documents"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True)
    user_id: Mapped[uuid.UUID | None] = mapped_column(UUID(as_uuid=True))
    s3_bucket: Mapped[str | None] = mapped_column(Text)
    source_filename: Mapped[str | None] = mapped_column(Text)
    game_name: Mapped[str] = mapped_column(
        Text,
        server_default=sql_text("'Untitled Game'"),
    )
    task_id: Mapped[str | None] = mapped_column(Text)
    is_ready: Mapped[bool] = mapped_column(
        Boolean,
        server_default=sql_text("false"),
        default=False,
    )
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


@dataclass(frozen=True)
class DocumentIndexRecord:
    document_id: str
    user_id: str
    s3_bucket: str
    source_filename: str
    chapters: list[ChapterSection]
    sub_chapters: list[SubChapterSection]


def db_uuid(value: str, namespace: str) -> uuid.UUID:
    try:
        return uuid.UUID(str(value))
    except ValueError:
        return uuid.uuid5(uuid.NAMESPACE_URL, f"ocr:{namespace}:{value}")


def engine():
    if not DATABASE_URL:
        raise RuntimeError("POSTGRES_* environment variables must be configured")
    return create_engine(DATABASE_URL, pool_pre_ping=True)


def ensure_schema() -> None:
    db_engine = engine()

    Base.metadata.create_all(db_engine)

    statements = [
        "ALTER TABLE documents ADD COLUMN IF NOT EXISTS source_filename TEXT",
        "ALTER TABLE documents ADD COLUMN IF NOT EXISTS game_name TEXT",
        """
        UPDATE documents
        SET game_name = COALESCE(NULLIF(game_name, ''), NULLIF(source_filename, ''), 'Untitled Game')
        WHERE game_name IS NULL OR game_name = ''
        """,
        "ALTER TABLE documents ALTER COLUMN game_name SET DEFAULT 'Untitled Game'",
        "ALTER TABLE documents ALTER COLUMN game_name SET NOT NULL",
        "ALTER TABLE documents ADD COLUMN IF NOT EXISTS task_id TEXT",
        "ALTER TABLE documents ADD COLUMN IF NOT EXISTS is_ready BOOLEAN DEFAULT false",
        "UPDATE documents SET is_ready = false WHERE is_ready IS NULL",
        "ALTER TABLE documents ALTER COLUMN is_ready SET DEFAULT false",
        "ALTER TABLE documents ALTER COLUMN is_ready SET NOT NULL",
        "CREATE INDEX IF NOT EXISTS documents_task_id_idx ON documents(task_id)",
        "CREATE INDEX IF NOT EXISTS documents_user_id_idx ON documents(user_id)",
    ]

    with db_engine.begin() as conn:
        for statement in statements:
            conn.execute(sql_text(statement))


def insert_document_index(record: DocumentIndexRecord) -> dict[str, int | str]:
    document_id = db_uuid(record.document_id, "document")
    user_id = db_uuid(record.user_id, "user")
    chapter_ids: dict[int, uuid.UUID] = {}

    ensure_schema()
    db_engine = engine()

    with Session(db_engine) as session:
        with session.begin():
            session.execute(delete(SubChapter).where(SubChapter.document_id == document_id))
            session.execute(delete(Chapter).where(Chapter.document_id == document_id))

            document = session.get(Document, document_id)
            if document is None:
                document = Document(
                    id=document_id,
                    game_name=record.source_filename or "Untitled Game",
                )
                session.add(document)
            document.user_id = user_id
            document.s3_bucket = record.s3_bucket
            document.source_filename = record.source_filename
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

    return {
        "document_id": str(document_id),
        "chapter_count": len(record.chapters),
        "sub_chapter_count": len(record.sub_chapters),
    }
