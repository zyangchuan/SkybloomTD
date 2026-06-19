from typing import Any

from .types import HeadingCandidate


def clean_outline_title(value: Any) -> str:
    if not isinstance(value, str):
        raise ValueError("Generated outline title must be a string")
    title = value.strip()
    if not title:
        raise ValueError("Generated outline title must not be empty")
    return title


def heading_for_id(
    candidates_by_id: dict[str, HeadingCandidate],
    heading_id: Any,
) -> HeadingCandidate:
    if not isinstance(heading_id, str):
        raise ValueError("Generated outline start_heading_id must be a string")

    try:
        return candidates_by_id[heading_id]
    except KeyError as exc:
        raise ValueError(
            f"Generated outline references unknown start_heading_id {heading_id!r}"
        ) from exc


def outline_items(outline: dict[str, Any]) -> tuple[list[Any], list[Any]]:
    if not isinstance(outline, dict):
        raise ValueError("Generated outline must be an object")

    raw_chapters = outline.get("chapters")
    if not isinstance(raw_chapters, list) or not raw_chapters:
        raise ValueError("Generated outline must include at least one chapter")

    raw_sub_chapters = outline.get("sub_chapters")
    if not isinstance(raw_sub_chapters, list):
        raise ValueError("Generated outline sub_chapters must be a list")

    return raw_chapters, raw_sub_chapters


def ensure_unique_heading_id(
    used_heading_ids: set[str],
    heading: HeadingCandidate,
) -> None:
    if heading.id in used_heading_ids:
        raise ValueError(
            f"Generated outline reused start_heading_id {heading.id!r}"
        )
    used_heading_ids.add(heading.id)


def normalized_outline_ref(
    raw_item: Any,
    candidates_by_id: dict[str, HeadingCandidate],
) -> dict[str, Any]:
    if not isinstance(raw_item, dict):
        raise ValueError("Generated outline item must be an object")

    return {
        "title": clean_outline_title(raw_item.get("title")),
        "heading": heading_for_id(
            candidates_by_id,
            raw_item.get("start_heading_id"),
        ),
    }


def normalized_outline_refs(
    raw_items: list[Any],
    candidates_by_id: dict[str, HeadingCandidate],
) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    used_heading_ids: set[str] = set()

    for raw_item in raw_items:
        item = normalized_outline_ref(raw_item, candidates_by_id)
        ensure_unique_heading_id(used_heading_ids, item["heading"])
        items.append(item)

    return items


def sort_by_heading_line(items: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return sorted(items, key=lambda item: item["heading"].line_number)


def chapter_line_range(
    chapters: list[dict[str, Any]],
    index: int,
) -> tuple[int, float]:
    chapter_start_line = chapters[index]["heading"].line_number
    chapter_end_line = (
        chapters[index + 1]["heading"].line_number - 1
        if index + 1 < len(chapters)
        else float("inf")
    )

    return chapter_start_line, chapter_end_line


def containing_chapter(
    chapters: list[dict[str, Any]],
    sub_chapter: dict[str, Any],
) -> dict[str, Any]:

    sub_start_line = sub_chapter["heading"].line_number

    for index, chapter in enumerate(chapters):
        chapter_start_line, chapter_end_line = chapter_line_range(
            chapters,
            index,
        )
        if chapter_start_line <= sub_start_line <= chapter_end_line:
            return chapter

    raise ValueError(
        "Generated sub-chapter start line must be inside a generated chapter"
    )


def attach_sub_chapters_to_chapters(
    chapters: list[dict[str, Any]],
    sub_chapters: list[dict[str, Any]],
) -> None:
    for chapter in chapters:
        chapter["sub_chapters"] = []

    for sub_chapter in sub_chapters:
        containing_chapter(chapters, sub_chapter)["sub_chapters"].append(sub_chapter)


def normalized_outline_chapters(
    candidates: list[HeadingCandidate],
    outline: dict[str, Any],
) -> list[dict[str, Any]]:

    raw_chapters, raw_sub_chapters = outline_items(outline)
    candidates_by_id = {candidate.id: candidate for candidate in candidates}

    chapters = sort_by_heading_line(
        normalized_outline_refs(raw_chapters, candidates_by_id)
    )

    sub_chapters = sort_by_heading_line(
        normalized_outline_refs(raw_sub_chapters, candidates_by_id)
    )

    attach_sub_chapters_to_chapters(chapters, sub_chapters)

    return chapters