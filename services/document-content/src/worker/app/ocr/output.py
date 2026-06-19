import re
import shutil
from dataclasses import dataclass
from pathlib import Path

from ..config import OUTPUT_ROOT

SAFE_PATH_PART_PATTERN = re.compile(r"[^A-Za-z0-9_.=-]+")

def safe_path_part(value: str) -> str:
    cleaned = SAFE_PATH_PART_PATTERN.sub("_", value.strip())
    return cleaned.strip("._") or "unknown"

def make_output_path(user_id: str, document_id: str) -> Path:
    path = OUTPUT_ROOT / "users" / user_id / "documents" / document_id
    if path.exists():
        shutil.rmtree(path)
    path.mkdir(parents=True, exist_ok=True)
    return path


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
