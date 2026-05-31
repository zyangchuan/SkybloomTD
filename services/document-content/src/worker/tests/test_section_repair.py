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


if __name__ == "__main__":
    unittest.main()
