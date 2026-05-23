import json
from collections.abc import Sequence
from typing import Any

from ..config import (
    SECTIONING_LLM_MAX_RETRIES,
    SECTIONING_LLM_MODEL,
    SECTIONING_LLM_TIMEOUT_SECONDS,
)


RESPONSE_SCHEMA = {
    "type": "object",
    "additionalProperties": False,
    "required": ["chapters"],
    "properties": {
        "chapters": {
            "type": "array",
            "items": {
                "type": "object",
                "additionalProperties": False,
                "required": ["title", "start_heading_id", "sub_chapters"],
                "properties": {
                    "title": {
                        "type": "string",
                        "description": "Clean normalized chapter title.",
                    },
                    "start_heading_id": {
                        "type": "string",
                        "description": (
                            "Input heading id where this chapter begins. Use the "
                            "earliest heading that belongs to this chapter."
                        ),
                    },
                    "sub_chapters": {
                        "type": "array",
                        "items": {
                            "type": "object",
                            "additionalProperties": False,
                            "required": ["title", "start_heading_id"],
                            "properties": {
                                "title": {
                                    "type": "string",
                                    "description": "Clean normalized sub-chapter title.",
                                },
                                "start_heading_id": {
                                    "type": "string",
                                    "description": (
                                        "Input heading id where this sub-chapter begins."
                                    ),
                                },
                            },
                        },
                    },
                },
            },
        }
    },
}


SYSTEM_PROMPT = """
You create a clean table of contents from noisy OCR Markdown headings.

You receive only the ordered heading list extracted from levels # to ###, not
the full document body. Produce a normalized outline with chapters and their
sub-chapters. Do not classify every input heading. Omit headings that are title
page text, contents labels, running/page headers, companion subtitles,
examples, definitions, theorem headings, malformed OCR garbage, decorative
titles, or anything that should not control slicing.

You may combine nearby headings into one clean title. For example, headings
like "Chapter 4" followed by "Integrals" can become "Chapter 4: Integrals".
Use start_heading_id to point to the earliest input heading that belongs to the
chapter or sub-chapter. Use only ids from the input list. Python will compute
all end lines from the chosen start ids, so do not output end lines.

Optimize for a coherent content page, not literal Markdown levels. PPStructureV3
may assign inconsistent levels, so a real sub-chapter may appear as "#" and a
chapter may appear as "##" or "###". Use ordering, numbering patterns,
neighboring headings, and repeated structure across the whole list.
""".strip()


def candidate_payload(candidate: Any, sequence_number: int) -> dict[str, Any]:
    return {
        "id": candidate.id,
        "sequence_number": sequence_number,
        "title": candidate.title,
        "markdown_level": candidate.level,
        "line_number": candidate.line_number,
        "range_start_line": candidate.line_number,
        "range_end_line": candidate.end_line,
        "previous_title": candidate.previous_title,
        "next_title": candidate.next_title,
    }


def generate_document_outline(candidates: Sequence[Any]) -> dict[str, Any]:
    from openai import OpenAI

    client = OpenAI(
        timeout=SECTIONING_LLM_TIMEOUT_SECONDS,
        max_retries=SECTIONING_LLM_MAX_RETRIES,
    )
    headings = [
        candidate_payload(candidate, sequence_number)
        for sequence_number, candidate in enumerate(candidates, start=1)
    ]

    response = client.chat.completions.create(
        model=SECTIONING_LLM_MODEL,
        messages=[
            {"role": "system", "content": SYSTEM_PROMPT},
            {
                "role": "user",
                "content": (
                    "Create a clean table of contents from the ordered OCR "
                    "Markdown headings below. Return normalized chapters and "
                    "sub-chapters with start_heading_id values only.\n\n"
                    f"{json.dumps({'headings': headings}, ensure_ascii=True)}"
                ),
            },
        ],
        response_format={
            "type": "json_schema",
            "json_schema": {
                "name": "document_outline",
                "strict": True,
                "schema": RESPONSE_SCHEMA,
            },
        },
    )

    content = response.choices[0].message.content
    if not content:
        raise ValueError("Sectioning LLM returned an empty response")

    parsed = json.loads(content)
    return parsed
