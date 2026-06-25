from dataclasses import dataclass


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
