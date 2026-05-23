import re
import shutil
import uuid
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from ..config import OUTPUT_ROOT

SAFE_PATH_PART_PATTERN = re.compile(r"[^A-Za-z0-9_.=-]+")


@dataclass(frozen=True)
class DocumentOutputPaths:
    document_dir: Path
    images_dir: Path
    markdown_file: Path


def safe_path_part(value: str) -> str:
    cleaned = SAFE_PATH_PART_PATTERN.sub("_", value.strip())
    return cleaned.strip("._") or "unknown"


def safe_relative_output_path(raw_path: str) -> Path:
    parts = []
    for part in Path(str(raw_path).replace("\\", "/")).parts:
        if part in ("", ".", "..", "/"):
            continue
        parts.append(part)

    if not parts:
        return Path("imgs") / f"image_{uuid.uuid4().hex}.png"

    return Path(*parts)


def document_output_paths(user_id: str, document_id: str) -> DocumentOutputPaths:
    document_dir = OUTPUT_ROOT / "users" / user_id / "documents" / document_id
    return DocumentOutputPaths(
        document_dir=document_dir,
        images_dir=document_dir / "imgs",
        markdown_file=document_dir / "output.md",
    )


def prepare_document_output(user_id: str, document_id: str) -> DocumentOutputPaths:
    paths = document_output_paths(user_id, document_id)
    if paths.document_dir.exists():
        shutil.rmtree(paths.document_dir)
    paths.images_dir.mkdir(parents=True, exist_ok=True)
    return paths


def remove_document_output(document_output_dir: Path) -> dict:
    if not document_output_dir.exists():
        return {
            "status": "skipped",
            "reason": "Output directory is already gone",
        }

    shutil.rmtree(document_output_dir)
    return {
        "status": "removed",
        "path": str(document_output_dir),
    }


def write_markdown(markdown_file: Path, final_output: str) -> None:
    with open(markdown_file, "w", encoding="utf-8") as f:
        f.write(final_output)


def write_markdown_images(
    document_output_dir: Path,
    markdown_images: list[dict[str, Any]],
) -> None:
    for item in markdown_images:
        if not item:
            continue

        for path, image in item.items():
            img_path = document_output_dir / safe_relative_output_path(path)
            img_path.parent.mkdir(parents=True, exist_ok=True)
            image.save(img_path)
