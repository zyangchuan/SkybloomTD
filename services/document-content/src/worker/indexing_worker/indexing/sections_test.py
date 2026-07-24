from . import sections as sections_module
from .sections import merge_small_sub_chapters
from .types import ChapterSection, SubChapterSection


def chapter() -> ChapterSection:
    return ChapterSection(
        index=1,
        title="Chapter 1",
        start_line=1,
        end_line=100,
    )


def test_merge_small_sub_chapter_into_next_section(monkeypatch):
    lines = [
        "# Chapter 1",
        "## Tiny intro",
        "short",
        "## Main lesson",
        *[
            "substantial content line " + str(index) + " " + ("x" * 120)
            for index in range(12)
        ],
    ]
    sub_chapters = [
        SubChapterSection(
            chapter_index=1,
            index=1,
            title="Tiny intro",
            start_line=2,
            end_line=3,
        ),
        SubChapterSection(
            chapter_index=1,
            index=2,
            title="Main lesson",
            start_line=4,
            end_line=len(lines),
        ),
    ]

    def decision(payload):
        assert payload["flagged_small_level"]["title"] == "Tiny intro"
        assert payload["next_level"]["title"] == "Main lesson"
        return {
            "merge_with": "next",
            "title": "Foundations of the Main Lesson",
            "reason": "The intro prepares the next lesson.",
        }

    monkeypatch.setattr(sections_module, "generate_merge_decision", decision)

    merged = merge_small_sub_chapters(lines, [chapter()], sub_chapters)

    assert merged == [
        SubChapterSection(
            chapter_index=1,
            index=1,
            title="Foundations of the Main Lesson",
            start_line=2,
            end_line=len(lines),
        )
    ]


def test_merge_small_tail_into_previous_section(monkeypatch):
    lines = [
        "# Chapter 1",
        "## Main lesson",
        *[
            "substantial content line " + str(index) + " " + ("x" * 120)
            for index in range(12)
        ],
        "## Tiny reflection",
        "short",
    ]
    sub_chapters = [
        SubChapterSection(
            chapter_index=1,
            index=1,
            title="Main lesson",
            start_line=2,
            end_line=14,
        ),
        SubChapterSection(
            chapter_index=1,
            index=2,
            title="Tiny reflection",
            start_line=15,
            end_line=16,
        ),
    ]

    monkeypatch.setattr(
        sections_module,
        "generate_merge_decision",
        lambda payload: {
            "merge_with": "previous",
            "title": "Main Lesson and Reflection",
            "reason": "The reflection closes the lesson.",
        },
    )

    merged = merge_small_sub_chapters(lines, [chapter()], sub_chapters)

    assert merged == [
        SubChapterSection(
            chapter_index=1,
            index=1,
            title="Main Lesson and Reflection",
            start_line=2,
            end_line=16,
        )
    ]


def test_invalid_merge_decision_leaves_small_section_unchanged(monkeypatch):
    lines = [
        "# Chapter 1",
        "## Tiny intro",
        "short",
        "## Main lesson",
        *[
            "substantial content line " + str(index) + " " + ("x" * 120)
            for index in range(12)
        ],
    ]
    sub_chapters = [
        SubChapterSection(
            chapter_index=1,
            index=1,
            title="Tiny intro",
            start_line=2,
            end_line=3,
        ),
        SubChapterSection(
            chapter_index=1,
            index=2,
            title="Main lesson",
            start_line=4,
            end_line=len(lines),
        ),
    ]

    monkeypatch.setattr(
        sections_module,
        "generate_merge_decision",
        lambda payload: {
            "merge_with": "next",
            "title": "",
            "reason": "Malformed title.",
        },
    )

    merged = merge_small_sub_chapters(lines, [chapter()], sub_chapters)

    assert merged == sub_chapters


def test_none_merge_decision_leaves_small_section_unchanged(monkeypatch):
    lines = [
        "# Chapter 1",
        "## Author's note",
        "short",
        "## Main lesson",
        *[
            "substantial content line " + str(index) + " " + ("x" * 120)
            for index in range(12)
        ],
    ]
    sub_chapters = [
        SubChapterSection(
            chapter_index=1,
            index=1,
            title="Author's note",
            start_line=2,
            end_line=3,
        ),
        SubChapterSection(
            chapter_index=1,
            index=2,
            title="Main lesson",
            start_line=4,
            end_line=len(lines),
        ),
    ]

    monkeypatch.setattr(
        sections_module,
        "generate_merge_decision",
        lambda payload: {
            "merge_with": "none",
            "title": "",
            "reason": "This is not useful learning content.",
        },
    )

    merged = merge_small_sub_chapters(lines, [chapter()], sub_chapters)

    assert merged == sub_chapters
