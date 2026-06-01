import unittest

from app.indexing import sections


MARKDOWN = """# Chapter 1
Intro text.
## Section 1.1
Section text.
## Section 1.2
More text.
# Chapter 2
Chapter text.
## Section 2.1
Final text.
"""


BAD_OUTLINE = {
    "chapters": [
        {
            "title": "Chapter 1",
            "start_heading_id": "h1",
            "sub_chapters": [
                {
                    "title": "Section 1.1",
                    "start_heading_id": "h999",
                }
            ],
        }
    ]
}


GOOD_OUTLINE = {
    "chapters": [
        {
            "title": "Chapter 1",
            "start_heading_id": "h1",
            "sub_chapters": [
                {
                    "title": "Section 1.1",
                    "start_heading_id": "h2",
                },
                {
                    "title": "Section 1.2",
                    "start_heading_id": "h3",
                },
            ],
        },
        {
            "title": "Chapter 2",
            "start_heading_id": "h4",
            "sub_chapters": [
                {
                    "title": "Section 2.1",
                    "start_heading_id": "h5",
                }
            ],
        },
    ]
}


CHAPTER_START_SUB_CHAPTER_OUTLINE = {
    "chapters": [
        {
            "title": "Chapter 1",
            "start_heading_id": "h1",
            "sub_chapters": [
                {
                    "title": "Chapter 1 Intro",
                    "start_heading_id": "h1",
                },
                {
                    "title": "Section 1.1",
                    "start_heading_id": "h2",
                },
            ],
        },
        {
            "title": "Chapter 2",
            "start_heading_id": "h4",
            "sub_chapters": [
                {
                    "title": "Chapter 2",
                    "start_heading_id": "h4",
                },
            ],
        },
    ]
}


WRONG_PARENT_OUTLINE = {
    "chapters": [
        {
            "title": "Chapter 1",
            "start_heading_id": "h1",
            "sub_chapters": [
                {
                    "title": "Section 2.1",
                    "start_heading_id": "h5",
                },
            ],
        },
        {
            "title": "Chapter 2",
            "start_heading_id": "h4",
            "sub_chapters": [],
        },
    ]
}


class SectionRepairTest(unittest.TestCase):
    def setUp(self):
        self.original_generate_outline = sections.generate_outline_with_llm
        self.original_max_repairs = sections.SECTIONING_OUTLINE_MAX_REPAIRS

    def tearDown(self):
        sections.generate_outline_with_llm = self.original_generate_outline
        sections.SECTIONING_OUTLINE_MAX_REPAIRS = self.original_max_repairs

    def test_repairs_outline_after_unknown_heading_id(self):
        calls = []
        outlines = [BAD_OUTLINE, GOOD_OUTLINE]
        sections.SECTIONING_OUTLINE_MAX_REPAIRS = 1

        def fake_generate(candidates, previous_outline=None, validation_error=None):
            calls.append(
                {
                    "candidate_ids": [candidate.id for candidate in candidates],
                    "previous_outline": previous_outline,
                    "validation_error": validation_error,
                }
            )
            return outlines.pop(0)

        sections.generate_outline_with_llm = fake_generate

        chapters, sub_chapters = sections.parse_markdown_sections(MARKDOWN)

        self.assertEqual(
            ["Chapter 1", "Chapter 2"],
            [chapter.title for chapter in chapters],
        )
        self.assertEqual(
            ["Section 1.1", "Section 1.2", "Section 2.1"],
            [sub_chapter.title for sub_chapter in sub_chapters],
        )
        self.assertEqual(["h1", "h2", "h3", "h4", "h5"], calls[0]["candidate_ids"])
        self.assertIsNone(calls[0]["previous_outline"])
        self.assertIsNone(calls[0]["validation_error"])
        self.assertEqual(BAD_OUTLINE, calls[1]["previous_outline"])
        self.assertIn("unknown heading id 'h999'", calls[1]["validation_error"])

    def test_raises_last_validation_error_after_repair_attempts_are_exhausted(self):
        calls = []
        sections.SECTIONING_OUTLINE_MAX_REPAIRS = 1

        def fake_generate(candidates, previous_outline=None, validation_error=None):
            calls.append(validation_error)
            return BAD_OUTLINE

        sections.generate_outline_with_llm = fake_generate

        with self.assertRaisesRegex(ValueError, "unknown heading id 'h999'"):
            sections.parse_markdown_sections(MARKDOWN)

        self.assertEqual(2, len(calls))
        self.assertIsNone(calls[0])
        self.assertIn("unknown heading id 'h999'", calls[1])

    def test_allows_sub_chapter_to_start_at_chapter_heading(self):
        lines = MARKDOWN.splitlines()
        candidates = sections.extract_heading_candidates(lines)

        _, sub_chapters = sections.build_sections_from_generated_outline(
            lines,
            candidates,
            CHAPTER_START_SUB_CHAPTER_OUTLINE,
        )

        self.assertEqual("Chapter 1 Intro", sub_chapters[0].title)
        self.assertEqual(1, sub_chapters[0].chapter_index)
        self.assertEqual(1, sub_chapters[0].start_line)
        self.assertEqual(2, sub_chapters[0].end_line)
        self.assertEqual("Chapter 2", sub_chapters[-1].title)
        self.assertEqual(2, sub_chapters[-1].chapter_index)

    def test_reassigns_sub_chapter_to_chapter_containing_its_heading(self):
        lines = MARKDOWN.splitlines()
        candidates = sections.extract_heading_candidates(lines)

        _, sub_chapters = sections.build_sections_from_generated_outline(
            lines,
            candidates,
            WRONG_PARENT_OUTLINE,
        )

        self.assertEqual(1, len(sub_chapters))
        self.assertEqual("Section 2.1", sub_chapters[0].title)
        self.assertEqual(2, sub_chapters[0].chapter_index)


if __name__ == "__main__":
    unittest.main()
