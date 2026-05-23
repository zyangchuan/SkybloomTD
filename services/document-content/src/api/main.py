import uvicorn
from fastapi import Depends, FastAPI, Header, HTTPException, UploadFile
from celery import Celery, chain
from pathlib import Path
import os
import re
import uuid

app = FastAPI(title="Document Content API")

REDIS_URL = os.getenv("REDIS_URL", "redis://redis:6379/0")
celery_app = Celery("document_content_worker", broker=REDIS_URL, backend=REDIS_URL)

TEMP_DIR = Path("/temp")
TEMP_DIR.mkdir(parents=True, exist_ok=True)

SAFE_PATH_PART_PATTERN = re.compile(r"[^A-Za-z0-9_.=-]+")


def safe_path_part(value: str) -> str:
    cleaned = SAFE_PATH_PART_PATTERN.sub("_", value.strip())
    return cleaned.strip("._") or "unknown"


def authenticated_user_id(
    x_authenticated_user_id: str | None = Header(
        default=None,
        alias="X-Authenticated-User-Id",
    ),
) -> str:
    if not x_authenticated_user_id:
        raise HTTPException(status_code=401, detail="Authentication required")
    return safe_path_part(x_authenticated_user_id)


@app.post("/upload-file")
async def upload_file(
    file: UploadFile,
    user_id: str = Depends(authenticated_user_id),
):

    document_id = uuid.uuid4().hex
    file_suffix = Path(file.filename or "").suffix or ".pdf"
    filename = Path(file.filename or f"input{file_suffix}").name
    file_path = TEMP_DIR / user_id / document_id / f"input{file_suffix}"
    file_path.parent.mkdir(parents=True, exist_ok=True)

    content = await file.read()

    with open(file_path, "wb") as f:
        f.write(content)
        f.flush()
        os.fsync(f.fileno())
    
    ocr_task_id = uuid.uuid4().hex
    upload_task_id = uuid.uuid4().hex
    index_task_id = uuid.uuid4().hex
    workflow = chain(
        celery_app.signature(
            "worker.process_ocr",
            args=[str(file_path), user_id, document_id, filename],
        ).set(task_id=ocr_task_id),
        celery_app.signature("worker.upload_ocr_output").set(task_id=upload_task_id),
        celery_app.signature("worker.index_ocr_output").set(task_id=index_task_id),
    )
    task = workflow.apply_async()

    return {
        "message": "Upload file success",
        "task_id": task.id,
        "ocr_task_id": ocr_task_id,
        "upload_task_id": upload_task_id,
        "index_task_id": index_task_id,
        "user_id": user_id,
        "document_id": document_id,
    }

if __name__ == "__main__":
    uvicorn.run(app, host="localhost", port=8000)
