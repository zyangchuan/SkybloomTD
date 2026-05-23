import importlib.util
from pathlib import Path
import sys
import types
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[1]
app_package = types.ModuleType("_test_worker_app")
app_package.__path__ = [str(ROOT / "app")]
indexing_package = types.ModuleType("_test_worker_app.indexing")
indexing_package.__path__ = [str(ROOT / "app" / "indexing")]
app_package.indexing = indexing_package
sys.modules["_test_worker_app"] = app_package
sys.modules["_test_worker_app.indexing"] = indexing_package


def load_module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


sections = load_module(
    "_test_worker_app.indexing.sections",
    ROOT / "app" / "indexing" / "sections.py",
)
indexing_package.sections = sections
extract_heading_candidates = sections.extract_heading_candidates
parse_markdown_sections = sections.parse_markdown_sections
PATCH_PREFIX = "_test_worker_app.indexing.sections"


class HeadingCandidateTests(unittest.TestCase):
    def test_extract_heading_candidates_includes_only_up_to_h3(self):
        lines = [
            "# First",
            "first body",
            "## Second",
            "### Third",
            "#### Fourth",
            "fourth body",
        ]

        candidates = extract_heading_candidates(lines)

        self.assertEqual([candidate.id for candidate in candidates], ["h1", "h2", "h3"])
        self.assertEqual([candidate.title for candidate in candidates], ["First", "Second", "Third"])
        self.assertEqual([candidate.level for candidate in candidates], [1, 2, 3])
        self.assertEqual([candidate.line_number for candidate in candidates], [1, 3, 4])
        self.assertEqual([candidate.end_line for candidate in candidates], [2, 3, 6])
        self.assertIsNone(candidates[0].previous_title)
        self.assertEqual(candidates[0].next_title, "Second")
        self.assertEqual(candidates[2].previous_title, "Second")
        self.assertIsNone(candidates[2].next_title)


class LlmSectionParsingTests(unittest.TestCase):
    def test_generated_outline_controls_inconsistent_heading_levels(self):
        markdown = "\n".join(
            [
                "## Chapter 4",
                "Read Thomas' Calculus.",
                "### 4.1 Antiderivatives",
                "Antiderivative body.",
                "# 4.4 Integration by Substitution",
                "Substitution body.",
                "#### Definition 4.1",
                "Definition body.",
                "## 2 CHAPTER 1 Linear Equations in Linear Algebra",
                "Running header body.",
            ]
        )

        outline = {
            "chapters": [
                {
                    "title": "Chapter 4: Integrals",
                    "start_heading_id": "h1",
                    "sub_chapters": [
                        {"title": "4.1 Antiderivatives", "start_heading_id": "h2"},
                        {
                            "title": "4.4 Integration by Substitution",
                            "start_heading_id": "h3",
                        },
                    ],
                }
            ]
        }

        with patch(
            f"{PATCH_PREFIX}.generate_outline_with_llm",
            return_value=outline,
        ) as generate_outline:
            chapters, sub_chapters = parse_markdown_sections(markdown)

        generate_outline.assert_called_once()
        sent_candidates = generate_outline.call_args.args[0]
        self.assertEqual(
            [candidate.title for candidate in sent_candidates],
            [
                "Chapter 4",
                "4.1 Antiderivatives",
                "4.4 Integration by Substitution",
                "2 CHAPTER 1 Linear Equations in Linear Algebra",
            ],
        )

        self.assertEqual(len(chapters), 1)
        self.assertEqual(chapters[0].title, "Chapter 4: Integrals")
        self.assertEqual((chapters[0].start_line, chapters[0].end_line), (1, 10))
        self.assertEqual(
            [sub_chapter.title for sub_chapter in sub_chapters],
            ["4.1 Antiderivatives", "4.4 Integration by Substitution"],
        )
        self.assertEqual(
            [(sub_chapter.start_line, sub_chapter.end_line) for sub_chapter in sub_chapters],
            [(3, 4), (5, 10)],
        )

    def test_generated_outline_merges_companion_chapter_titles(self):
        markdown = "\n".join(
            [
                "## Chapter 3",
                "## Applications of Differentiation",
                "### 3.1 Tangents and Normals",
                "Tangent body.",
                "#  3.2 Increasing and Decreasing Functions",
                "Increasing body.",
                "## Chapter 4",
                "## Integrals",
                "### 4.1 Antiderivatives",
                "Antiderivative body.",
            ]
        )
        outline = {
            "chapters": [
                {
                    "title": "Chapter 3: Applications of Differentiation",
                    "start_heading_id": "h1",
                    "sub_chapters": [
                        {
                            "title": "3.1 Tangents and Normals",
                            "start_heading_id": "h3",
                        },
                        {
                            "title": "3.2 Increasing and Decreasing Functions",
                            "start_heading_id": "h4",
                        },
                    ],
                },
                {
                    "title": "Chapter 4: Integrals",
                    "start_heading_id": "h5",
                    "sub_chapters": [
                        {"title": "4.1 Antiderivatives", "start_heading_id": "h7"},
                    ],
                },
            ]
        }

        with patch(
            f"{PATCH_PREFIX}.generate_outline_with_llm",
            return_value=outline,
        ):
            chapters, sub_chapters = parse_markdown_sections(markdown)

        self.assertEqual(
            [chapter.title for chapter in chapters],
            ["Chapter 3: Applications of Differentiation", "Chapter 4: Integrals"],
        )
        self.assertEqual(
            [(chapter.start_line, chapter.end_line) for chapter in chapters],
            [(1, 6), (7, 10)],
        )
        self.assertEqual(
            [sub_chapter.title for sub_chapter in sub_chapters],
            [
                "3.1 Tangents and Normals",
                "3.2 Increasing and Decreasing Functions",
                "4.1 Antiderivatives",
            ],
        )
        self.assertEqual(
            [sub_chapter.chapter_index for sub_chapter in sub_chapters],
            [1, 1, 2],
        )

    def test_ignored_front_matter_does_not_create_synthetic_chapter(self):
        markdown = "\n".join(
            [
                "# Course Title",
                "Title page body.",
                "## Contents",
                "Contents body.",
                "## Chapter 1",
                "Chapter body.",
                "### 1.1 First Section",
                "Section body.",
            ]
        )
        outline = {
            "chapters": [
                {
                    "title": "Chapter 1",
                    "start_heading_id": "h3",
                    "sub_chapters": [
                        {"title": "1.1 First Section", "start_heading_id": "h4"},
                    ],
                }
            ]
        }

        with patch(
            f"{PATCH_PREFIX}.generate_outline_with_llm",
            return_value=outline,
        ):
            chapters, sub_chapters = parse_markdown_sections(markdown)

        self.assertEqual([chapter.title for chapter in chapters], ["Chapter 1"])
        self.assertEqual((chapters[0].start_line, chapters[0].end_line), (5, 8))
        self.assertEqual([sub_chapter.title for sub_chapter in sub_chapters], ["1.1 First Section"])

    def test_llm_classification_supports_unnumbered_documents(self):
        markdown = "\n".join(
            [
                "# Foundations",
                "Intro body.",
                "## Motivation",
                "Motivation body.",
                "## Method",
                "Method body.",
            ]
        )
        outline = {
            "chapters": [
                {
                    "title": "Foundations",
                    "start_heading_id": "h1",
                    "sub_chapters": [
                        {"title": "Motivation", "start_heading_id": "h2"},
                        {"title": "Method", "start_heading_id": "h3"},
                    ],
                }
            ]
        }

        with patch(
            f"{PATCH_PREFIX}.generate_outline_with_llm",
            return_value=outline,
        ):
            chapters, sub_chapters = parse_markdown_sections(markdown)

        self.assertEqual([chapter.title for chapter in chapters], ["Foundations"])
        self.assertEqual(
            [sub_chapter.title for sub_chapter in sub_chapters],
            ["Motivation", "Method"],
        )
        self.assertEqual(
            [(sub_chapter.start_line, sub_chapter.end_line) for sub_chapter in sub_chapters],
            [(3, 4), (5, 6)],
        )

    def test_invalid_llm_ids_raise_sectioning_error(self):
        markdown = "\n".join(
            [
                "# A",
                "A body.",
                "## B",
                "B body.",
                "### C",
                "C body.",
            ]
        )

        with self.assertRaisesRegex(ValueError, "unknown heading id 'unknown'"):
            with patch(
                f"{PATCH_PREFIX}.generate_outline_with_llm",
                return_value={
                    "chapters": [
                        {
                            "title": "A",
                            "start_heading_id": "unknown",
                            "sub_chapters": [],
                        }
                    ]
                },
            ):
                parse_markdown_sections(markdown)

    def test_no_llm_chapters_raises_sectioning_error(self):
        markdown = "\n".join(
            [
                "# A",
                "A body.",
                "## B",
                "B body.",
            ]
        )
        outline = {"chapters": []}

        with self.assertRaisesRegex(ValueError, "at least one chapter"):
            with patch(
                f"{PATCH_PREFIX}.generate_outline_with_llm",
                return_value=outline,
            ):
                parse_markdown_sections(markdown)

    def test_missing_classifiable_headings_raises_sectioning_error(self):
        markdown = "\n".join(
            [
                "Plain title",
                "Body text.",
                "#### Too Deep",
                "More body.",
            ]
        )

        with self.assertRaisesRegex(ValueError, "no classifiable headings"):
            with patch(f"{PATCH_PREFIX}.generate_outline_with_llm") as generate_outline:
                parse_markdown_sections(markdown)

        generate_outline.assert_not_called()

if __name__ == "__main__":
    unittest.main()
