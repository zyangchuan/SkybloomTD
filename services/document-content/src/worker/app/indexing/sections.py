import re
from dataclasses import dataclass

HEADING_PATTERN = re.compile(r"^(#{1,6})\s+(.+?)\s*#*\s*$")
CLASSIFIABLE_HEADING_MAX_LEVEL = 3


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


def build_sections_from_headings(
    lines: list[str],
    candidates: list[HeadingCandidate],
) -> tuple[list[ChapterSection], list[SubChapterSection]]:
    chapter_level = min(candidate.level for candidate in candidates)
    chapter_headings = [
        candidate for candidate in candidates if candidate.level == chapter_level
    ]
    chapters: list[ChapterSection] = []
    sub_chapters: list[SubChapterSection] = []

    for chapter_offset, chapter_heading in enumerate(chapter_headings):
        chapter_index = chapter_offset + 1
        chapter_start_line = chapter_heading.line_number
        next_chapter_start_line = (
            chapter_headings[chapter_offset + 1].line_number
            if chapter_offset + 1 < len(chapter_headings)
            else len(lines) + 1
        )
        chapter_end_line = max(chapter_start_line, next_chapter_start_line - 1)

        if chapter_start_line >= next_chapter_start_line:
            raise ValueError("Generated chapter start lines must be strictly increasing")

        chapters.append(
            ChapterSection(
                index=chapter_index,
                title=chapter_heading.title,
                start_line=chapter_start_line,
                end_line=chapter_end_line,
            )
        )

        sub_chapter_headings = [
            candidate
            for candidate in candidates
            if chapter_start_line < candidate.line_number < next_chapter_start_line
            and candidate.level > chapter_level
        ]

        if not sub_chapter_headings:
            sub_chapters.append(
                SubChapterSection(
                    chapter_index=chapter_index,
                    index=1,
                    title=chapter_heading.title,
                    start_line=chapter_start_line,
                    end_line=chapter_end_line,
                )
            )
            continue

        for sub_offset, sub_heading in enumerate(sub_chapter_headings):
            sub_start_line = sub_heading.line_number

            next_sub_start_line = (
                sub_chapter_headings[sub_offset + 1].line_number
                if sub_offset + 1 < len(sub_chapter_headings)
                else next_chapter_start_line
            )
            sub_end_line = max(sub_start_line, next_sub_start_line - 1)
            sub_chapters.append(
                SubChapterSection(
                    chapter_index=chapter_index,
                    index=sub_offset + 1,
                    title=sub_heading.title,
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

    return build_sections_from_headings(
        lines,
        candidates,
    )
