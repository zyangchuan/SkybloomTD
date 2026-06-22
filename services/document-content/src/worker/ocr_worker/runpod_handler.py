from __future__ import annotations

import tempfile
import time
import urllib.request
from pathlib import Path
from typing import Any

from ocr_worker.processing import run_ocr


def handler(event: dict[str, Any]) -> dict[str, Any]:
    payload = event.get("input", event)
    job_id = _required(payload, "job_id")
    source_url = _required(payload, "source_url")
    markdown_put_url = _required(payload, "markdown_put_url")

    started = time.monotonic()
    with tempfile.TemporaryDirectory() as temp_dir:
        source_path = Path(temp_dir) / "source.pdf"
        _download(source_url, source_path)
        markdown = run_ocr(source_path)
        _upload(markdown_put_url, markdown.encode("utf-8"))

    return {
        "status": "ocr_completed",
        "job_id": job_id,
        "duration_seconds": round(time.monotonic() - started, 3),
    }


def _required(payload: dict[str, Any], key: str) -> str:
    value = payload.get(key)
    if not isinstance(value, str) or value == "":
        raise ValueError(f"missing required input: {key}")
    return value


def _download(url: str, destination: Path) -> None:
    with urllib.request.urlopen(url, timeout=120) as response:
        destination.write_bytes(response.read())


def _upload(url: str, body: bytes) -> None:
    request = urllib.request.Request(url, data=body, method="PUT")
    with urllib.request.urlopen(request, timeout=120) as response:
        response.read()


if __name__ == "__main__":
    import runpod

    runpod.serverless.start({"handler": handler})
