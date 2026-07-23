import json
from typing import Any

from indexing_worker.config import (
    SECTIONING_LLM_MAX_RETRIES,
    SECTIONING_LLM_MODEL,
    SECTIONING_LLM_TIMEOUT_SECONDS,
)


RESPONSE_SCHEMA = {
    "type": "object",
    "additionalProperties": False,
    "required": ["merge_with", "title", "reason"],
    "properties": {
        "merge_with": {
            "type": "string",
            "enum": ["previous", "next", "none"],
            "description": (
                "Choose previous, next, or none for the flagged small learning level."
            ),
        },
        "title": {
            "type": "string",
            "description": (
                "Concise shared title for the merged learning level. Use an empty "
                "string when merge_with is none."
            ),
        },
        "reason": {
            "type": "string",
            "description": "Brief reason for the decision.",
        },
    },
}


SYSTEM_PROMPT = """
You decide whether a small document sub-chapter should be merged with a nearby
sub-chapter to create a better learning level.

You receive the flagged small level, plus its previous and next neighbors when
they exist. Choose:
- previous: if the small level belongs conceptually with the previous level.
- next: if the small level belongs conceptually with the next level.
- none: if it should remain separate, is front/back matter, is source metadata,
  or neither neighbor is a good learning match.

Only merge when the combined result would be more useful to a learner. Do not
merge just because a section is short. Prefer the neighbor with the strongest
conceptual relationship, not necessarily the closest or longest section.

When merging, produce a specific common title that describes the shared learning
objective. Avoid generic titles such as "Related Concepts", "Combined Lesson",
"Overview", "Introduction", or "Summary" unless that is truly the document's
specific subject.
""".strip()


def generate_sub_chapter_merge_decision(payload: dict[str, Any]) -> dict[str, Any]:
    from openai import OpenAI

    client = OpenAI(
        timeout=SECTIONING_LLM_TIMEOUT_SECONDS,
        max_retries=SECTIONING_LLM_MAX_RETRIES,
    )

    response = client.chat.completions.create(
        model=SECTIONING_LLM_MODEL,
        messages=[
            {"role": "system", "content": SYSTEM_PROMPT},
            {
                "role": "user",
                "content": (
                    "Decide whether to merge the flagged small level.\n\n"
                    f"{json.dumps(payload, ensure_ascii=True)}"
                ),
            },
        ],
        response_format={
            "type": "json_schema",
            "json_schema": {
                "name": "sub_chapter_merge_decision",
                "strict": True,
                "schema": RESPONSE_SCHEMA,
            },
        },
    )

    content = response.choices[0].message.content
    if not content:
        raise ValueError("Merge LLM returned an empty response")

    return json.loads(content)
