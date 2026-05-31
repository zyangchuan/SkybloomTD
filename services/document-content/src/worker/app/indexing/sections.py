import logging
import re
from dataclasses import dataclass
from typing import Any

from ..config import SECTIONING_OUTLINE_MAX_REPAIRS

HEADING_PATTERN = re.compile(r"^(#{1,6})\s+(.+?)\s*#*\s*$")
CLASSIFIABLE_HEADING_MAX_LEVEL = 3
LOG = logging.getLogger(__name__)


@dataclass(frozen=True)
class ChapterSection:
    index: int
    title: str
    start_line: int
    end_line: int


@dataclass(frozen=True)
class SubChapterSection:
    chapter_index: int
    index: int
    title: str
    start_line: int
    end_line: int


@dataclass(frozen=True)
class HeadingCandidate:
    id: str
    title: str
    level: int
    line_number: int
    end_line: int
    previous_title: str | None
    next_title: str | None


def heading_title(line: str) -> tuple[int, str] | None:
    match = HEADING_PATTERN.match(line.strip())
    if not match:
        return None
    return len(match.group(1)), match.group(2).strip()


def extract_heading_candidates(lines: list[str]) -> list[HeadingCandidate]:
    headings: list[dict[str, int | str]] = []

    for line_number, line in enumerate(lines, start=1):
        heading = heading_title(line)
        if not heading:
            continue

        level, title = heading
        if level > CLASSIFIABLE_HEADING_MAX_LEVEL:
            continue

        headings.append(
            {
                "id": f"h{len(headings) + 1}",
                "title": title,
                "level": level,
                "line_number": line_number,
            }
        )

    candidates: list[HeadingCandidate] = []
    for index, heading in enumerate(headings):
        previous_title = str(headings[index - 1]["title"]) if index > 0 else None
        next_title = (
            str(headings[index + 1]["title"])
            if index + 1 < len(headings)
            else None
        )
        end_line = (
            int(headings[index + 1]["line_number"]) - 1
            if index + 1 < len(headings)
            else len(lines)
        )
        candidates.append(
            HeadingCandidate(
                id=heading["id"],
                title=heading["title"],
                level=heading["level"],
                line_number=heading["line_number"],
                end_line=end_line,
                previous_title=previous_title,
                next_title=next_title,
            )
        )

    return candidates


def generate_outline_with_llm(
    candidates: list[HeadingCandidate],
    previous_outline: dict[str, Any] | None = None,
    validation_error: str | None = None,
) -> dict[str, Any]:
    from .sectioning_llm import generate_document_outline

    return generate_document_outline(
        candidates,
        previous_outline=previous_outline,
        validation_error=validation_error,
    )


def clean_outline_title(value: Any, field_name: str) -> str:
    if not isinstance(value, str):
        raise ValueError(f"Generated outline {field_name} must be a string")
    title = value.strip()
    if not title:
        raise ValueError(f"Generated outline {field_name} must not be empty")
    return title


def heading_for_id(
    candidates_by_id: dict[str, HeadingCandidate],
    heading_id: Any,
    field_name: str,
) -> HeadingCandidate:
    if not isinstance(heading_id, str):
        raise ValueError(f"Generated outline {field_name} must be a string")
    try:
        return candidates_by_id[heading_id]
    except KeyError as exc:
        raise ValueError(
            f"Generated outline {field_name} references unknown heading id {heading_id!r}"
        ) from exc


def normalized_outline_chapters(
    candidates: list[HeadingCandidate],
    outline: dict[str, Any],
) -> list[dict[str, Any]]:
    if not isinstance(outline, dict):
        raise ValueError("Generated outline must be an object")

    raw_chapters = outline.get("chapters")
    if not isinstance(raw_chapters, list) or not raw_chapters:
        raise ValueError("Generated outline must include at least one chapter")

    candidates_by_id = {candidate.id: candidate for candidate in candidates}
    normalized_chapters: list[dict[str, Any]] = []
    used_chapter_ids: set[str] = set()

    for chapter_index, raw_chapter in enumerate(raw_chapters, start=1):
        if not isinstance(raw_chapter, dict):
            raise ValueError(f"Generated chapter {chapter_index} must be an object")

        title = clean_outline_title(raw_chapter.get("title"), "chapter.title")
        start_heading = heading_for_id(
            candidates_by_id,
            raw_chapter.get("start_heading_id"),
            "chapter.start_heading_id",
        )
        if start_heading.id in used_chapter_ids:
            raise ValueError(
                f"Generated outline reused chapter heading id {start_heading.id!r}"
            )
        used_chapter_ids.add(start_heading.id)

        raw_sub_chapters = raw_chapter.get("sub_chapters", [])
        if not isinstance(raw_sub_chapters, list):
            raise ValueError("Generated chapter.sub_chapters must be a list")

        sub_chapters: list[dict[str, Any]] = []
        used_sub_chapter_ids: set[str] = set()
        for sub_index, raw_sub_chapter in enumerate(raw_sub_chapters, start=1):
            if not isinstance(raw_sub_chapter, dict):
                raise ValueError(
                    f"Generated sub-chapter {chapter_index}.{sub_index} "
                    "must be an object"
                )

            sub_title = clean_outline_title(
                raw_sub_chapter.get("title"),
                "sub_chapter.title",
            )
            sub_start_heading = heading_for_id(
                candidates_by_id,
                raw_sub_chapter.get("start_heading_id"),
                "sub_chapter.start_heading_id",
            )
            if sub_start_heading.id == start_heading.id:
                raise ValueError(
                    "Generated sub-chapter cannot reuse its chapter start heading id"
                )
            if sub_start_heading.id in used_sub_chapter_ids:
                raise ValueError(
                    f"Generated outline reused sub-chapter heading id "
                    f"{sub_start_heading.id!r}"
                )
            used_sub_chapter_ids.add(sub_start_heading.id)
            sub_chapters.append(
                {
                    "title": sub_title,
                    "heading": sub_start_heading,
                }
            )

        normalized_chapters.append(
            {
                "title": title,
                "heading": start_heading,
                "sub_chapters": sorted(
                    sub_chapters,
                    key=lambda item: item["heading"].line_number,
                ),
            }
        )

    normalized_chapters.sort(key=lambda item: item["heading"].line_number)
    return normalized_chapters


def build_sections_from_generated_outline(
    lines: list[str],
    candidates: list[HeadingCandidate],
    outline: dict[str, Any],
) -> tuple[list[ChapterSection], list[SubChapterSection]]:
    outline_chapters = normalized_outline_chapters(candidates, outline)
    chapters: list[ChapterSection] = []
    sub_chapters: list[SubChapterSection] = []

    for chapter_offset, chapter in enumerate(outline_chapters):
        chapter_index = chapter_offset + 1
        chapter_start_line = chapter["heading"].line_number
        next_chapter_start_line = (
            outline_chapters[chapter_offset + 1]["heading"].line_number
            if chapter_offset + 1 < len(outline_chapters)
            else len(lines) + 1
        )
        chapter_end_line = max(chapter_start_line, next_chapter_start_line - 1)

        if chapter_start_line >= next_chapter_start_line:
            raise ValueError("Generated chapter start lines must be strictly increasing")

        chapters.append(
            ChapterSection(
                index=chapter_index,
                title=chapter["title"],
                start_line=chapter_start_line,
                end_line=chapter_end_line,
            )
        )

        sub_chapters_for_chapter = chapter["sub_chapters"]
        for sub_offset, sub_chapter in enumerate(sub_chapters_for_chapter):
            sub_start_line = sub_chapter["heading"].line_number
            if not chapter_start_line < sub_start_line < next_chapter_start_line:
                raise ValueError(
                    "Generated sub-chapter start line must be inside its chapter"
                )

            next_sub_start_line = (
                sub_chapters_for_chapter[sub_offset + 1]["heading"].line_number
                if sub_offset + 1 < len(sub_chapters_for_chapter)
                else next_chapter_start_line
            )
            sub_end_line = max(sub_start_line, next_sub_start_line - 1)
            sub_chapters.append(
                SubChapterSection(
                    chapter_index=chapter_index,
                    index=sub_offset + 1,
                    title=sub_chapter["title"],
                    start_line=sub_start_line,
                    end_line=min(sub_end_line, chapter_end_line),
                )
            )

    return chapters, sub_chapters


def parse_markdown_sections(
    markdown: str,
) -> tuple[list[ChapterSection], list[SubChapterSection]]:
    lines = markdown.splitlines() or [""]
    candidates = extract_heading_candidates(lines)

    if not candidates:
        raise ValueError("Cannot section markdown because no classifiable headings were found")

    previous_outline: dict[str, Any] | None = None
    validation_error: str | None = None
    max_attempts = max(0, SECTIONING_OUTLINE_MAX_REPAIRS) + 1

    for attempt in range(1, max_attempts + 1):
        outline = generate_outline_with_llm(
            candidates,
            previous_outline=previous_outline,
            validation_error=validation_error,
        )
        try:
            return build_sections_from_generated_outline(
                lines,
                candidates,
                outline,
            )
        except ValueError as exc:
            if attempt >= max_attempts:
                raise
            validation_error = str(exc)
            previous_outline = outline
            LOG.info(
                "generated outline failed validation; requesting repair "
                "attempt=%s max_attempts=%s error=%s",
                attempt + 1,
                max_attempts,
                validation_error,
            )

    raise ValueError("Generated outline repair loop ended without a valid outline")
