import logging
import re
from typing import Any

from indexing_worker.config import (
    MIN_SUB_CHAPTER_CONTENT_CHARS,
    MIN_SUB_CHAPTER_CONTENT_LINES,
    SECTIONING_OUTLINE_MAX_REPAIRS,
)
from .outline_validation import normalized_outline_chapters
from .types import ChapterSection, HeadingCandidate, SubChapterSection

HEADING_PATTERN = re.compile(r"^(#{1,6})\s+(.+?)\s*#*\s*$")
CLASSIFIABLE_HEADING_MAX_LEVEL = 3
MERGE_CONTEXT_MAX_CHARS = 2400
LOG = logging.getLogger(__name__)


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
            if not chapter_start_line <= sub_start_line < next_chapter_start_line:
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


def content_lines_for_section(lines: list[str], section: SubChapterSection) -> list[str]:
    start_index = max(section.start_line - 1, 0)
    end_index = min(section.end_line, len(lines))
    content: list[str] = []
    for line in lines[start_index:end_index]:
        stripped = line.strip()
        if not stripped:
            continue
        if heading_title(stripped):
            continue
        content.append(stripped)
    return content


def is_small_sub_chapter(lines: list[str], section: SubChapterSection) -> bool:
    content = content_lines_for_section(lines, section)
    content_chars = sum(len(line) for line in content)
    return (
        len(content) < MIN_SUB_CHAPTER_CONTENT_LINES
        or content_chars < MIN_SUB_CHAPTER_CONTENT_CHARS
    )


def section_excerpt(lines: list[str], section: SubChapterSection | None) -> str | None:
    if section is None:
        return None

    content = "\n".join(content_lines_for_section(lines, section))
    if len(content) <= MERGE_CONTEXT_MAX_CHARS:
        return content
    return content[:MERGE_CONTEXT_MAX_CHARS].rstrip()


def section_payload(
    lines: list[str],
    section: SubChapterSection | None,
) -> dict[str, Any] | None:
    if section is None:
        return None
    return {
        "title": section.title,
        "start_line": section.start_line,
        "end_line": section.end_line,
        "content_excerpt": section_excerpt(lines, section),
    }


def chapter_title_by_index(chapters: list[ChapterSection]) -> dict[int, str]:
    return {chapter.index: chapter.title for chapter in chapters}


def generate_merge_decision(payload: dict[str, Any]) -> dict[str, Any]:
    from .merge_llm import generate_sub_chapter_merge_decision

    return generate_sub_chapter_merge_decision(payload)


def merge_sub_chapter_pair(
    left: SubChapterSection,
    right: SubChapterSection,
    title: str,
) -> SubChapterSection:
    return SubChapterSection(
        chapter_index=left.chapter_index,
        index=left.index,
        title=title,
        start_line=min(left.start_line, right.start_line),
        end_line=max(left.end_line, right.end_line),
    )


def reindex_sub_chapters(sub_chapters: list[SubChapterSection]) -> list[SubChapterSection]:
    reindexed: list[SubChapterSection] = []
    current_chapter_index: int | None = None
    next_index = 1

    for section in sorted(
        sub_chapters,
        key=lambda item: (item.chapter_index, item.start_line, item.end_line),
    ):
        if section.chapter_index != current_chapter_index:
            current_chapter_index = section.chapter_index
            next_index = 1
        reindexed.append(
            SubChapterSection(
                chapter_index=section.chapter_index,
                index=next_index,
                title=section.title,
                start_line=section.start_line,
                end_line=section.end_line,
            )
        )
        next_index += 1

    return reindexed


def validated_merge_target(
    decision: dict[str, Any],
    section: SubChapterSection,
    previous_section: SubChapterSection | None,
    next_section: SubChapterSection | None,
) -> tuple[str, str] | None:
    merge_with = decision.get("merge_with")
    if merge_with not in {"previous", "next", "none"}:
        return None
    if merge_with == "none":
        return ("none", "")

    title = str(decision.get("title", "")).strip()
    if not title:
        return None
    if merge_with == "previous" and previous_section is None:
        return None
    if merge_with == "next" and next_section is None:
        return None
    neighbor = previous_section if merge_with == "previous" else next_section
    if neighbor is None or neighbor.chapter_index != section.chapter_index:
        return None
    merged_start_line = min(section.start_line, neighbor.start_line)
    merged_end_line = max(section.end_line, neighbor.end_line)
    if merged_start_line > merged_end_line:
        return None
    return (merge_with, title)


def merge_with_llm_decision(
    lines: list[str],
    chapter_title: str | None,
    sections: list[SubChapterSection],
    index: int,
) -> tuple[list[SubChapterSection], bool]:
    section = sections[index]
    previous_section = sections[index - 1] if index > 0 else None
    next_section = sections[index + 1] if index + 1 < len(sections) else None

    if previous_section is None and next_section is None:
        return sections, False

    payload = {
        "chapter_title": chapter_title,
        "flagged_small_level": section_payload(lines, section),
        "previous_level": section_payload(lines, previous_section),
        "next_level": section_payload(lines, next_section),
    }
    try:
        decision = generate_merge_decision(payload)
    except Exception:
        LOG.exception(
            "small sub-chapter merge decision failed; leaving section unchanged "
            "chapter_index=%s section_index=%s start_line=%s end_line=%s",
            section.chapter_index,
            section.index,
            section.start_line,
            section.end_line,
        )
        return sections, False

    validated = validated_merge_target(decision, section, previous_section, next_section)
    if validated is None:
        LOG.info(
            "small sub-chapter merge decision failed validation; leaving unchanged "
            "chapter_index=%s section_index=%s decision=%s",
            section.chapter_index,
            section.index,
            decision,
        )
        return sections, False

    merge_with, title = validated
    if merge_with == "none":
        return sections, False
    if merge_with == "previous":
        return [
            *sections[: index - 1],
            merge_sub_chapter_pair(previous_section, section, title),
            *sections[index + 1 :],
        ], True

    if next_section is None:
        return sections, False
    return [
        *sections[:index],
        merge_sub_chapter_pair(section, next_section, title),
        *sections[index + 2 :],
    ], True


def merge_small_sub_chapters(
    lines: list[str],
    chapters: list[ChapterSection],
    sub_chapters: list[SubChapterSection],
) -> list[SubChapterSection]:
    by_chapter: dict[int, list[SubChapterSection]] = {}
    for section in sub_chapters:
        by_chapter.setdefault(section.chapter_index, []).append(section)

    chapter_titles = chapter_title_by_index(chapters)
    merged_all: list[SubChapterSection] = []
    for chapter_index in sorted(by_chapter):
        sections = sorted(by_chapter[chapter_index], key=lambda item: item.start_line)
        attempted: set[tuple[int, int, int]] = set()
        while len(sections) > 1:
            small_index = next(
                (
                    index
                    for index, section in enumerate(sections)
                    if is_small_sub_chapter(lines, section)
                    and (
                        section.chapter_index,
                        section.start_line,
                        section.end_line,
                    )
                    not in attempted
                ),
                None,
            )
            if small_index is None:
                break
            section = sections[small_index]
            attempted.add((section.chapter_index, section.start_line, section.end_line))
            sections, merged = merge_with_llm_decision(
                lines,
                chapter_titles.get(chapter_index),
                sections,
                small_index,
            )
            if merged:
                attempted.clear()

        merged_all.extend(sections)

    return reindex_sub_chapters(merged_all)


def parse_markdown_sections(
    markdown: str,
) -> tuple[list[ChapterSection], list[SubChapterSection]]:
    lines = markdown.splitlines()
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
            chapters, sub_chapters = build_sections_from_generated_outline(
                lines,
                candidates,
                outline,
            )
            return chapters, merge_small_sub_chapters(lines, chapters, sub_chapters)
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
