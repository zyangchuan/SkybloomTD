from __future__ import annotations

import json
import time
import urllib.error
import urllib.request
from typing import Any

from ocr_coordinator.config import (
    RUNPOD_API_KEY,
    RUNPOD_ENDPOINT_ID,
    RUNPOD_JOB_TIMEOUT_SECONDS,
    RUNPOD_POLL_INTERVAL_SECONDS,
)


class RunPodError(RuntimeError):
    pass


def submit_ocr_job(payload: dict[str, Any]) -> dict[str, Any]:
    if RUNPOD_API_KEY is None or RUNPOD_ENDPOINT_ID is None:
        raise RunPodError("RunPod is not configured")

    body = {"input": payload}
    started = _request_json("POST", _endpoint_url("run"), body)
    job_id = started.get("id")
    if not job_id:
        raise RunPodError(f"RunPod response missing job id: {started}")

    deadline = time.monotonic() + RUNPOD_JOB_TIMEOUT_SECONDS
    while time.monotonic() < deadline:
        status = _request_json("GET", _endpoint_url(f"status/{job_id}"), None)
        state = status.get("status")
        if state == "COMPLETED":
            output = status.get("output")
            if not isinstance(output, dict):
                raise RunPodError(f"RunPod completed without object output: {status}")
            return output
        if state in {"FAILED", "CANCELLED", "TIMED_OUT"}:
            raise RunPodError(f"RunPod job {job_id} ended with status {state}: {status}")
        time.sleep(RUNPOD_POLL_INTERVAL_SECONDS)

    raise RunPodError(f"RunPod job {job_id} timed out after {RUNPOD_JOB_TIMEOUT_SECONDS}s")


def _endpoint_url(operation: str) -> str:
    return f"https://api.runpod.ai/v2/{RUNPOD_ENDPOINT_ID}/{operation}"


def _request_json(method: str, url: str, body: dict[str, Any] | None) -> dict[str, Any]:
    data = None if body is None else json.dumps(body).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=data,
        method=method,
        headers={
            "authorization": f"Bearer {RUNPOD_API_KEY}",
            "content-type": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            response_body = response.read().decode("utf-8")
    except urllib.error.HTTPError as exc:
        details = exc.read().decode("utf-8", errors="replace")
        raise RunPodError(f"RunPod HTTP {exc.code}: {details}") from exc
    except urllib.error.URLError as exc:
        raise RunPodError(f"RunPod request failed: {exc}") from exc

    try:
        parsed = json.loads(response_body)
    except json.JSONDecodeError as exc:
        raise RunPodError(f"RunPod returned invalid JSON: {response_body}") from exc
    if not isinstance(parsed, dict):
        raise RunPodError(f"RunPod returned non-object JSON: {parsed}")
    return parsed
