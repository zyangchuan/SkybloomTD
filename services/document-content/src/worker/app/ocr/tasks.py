import traceback

from ..config import INPUT_ROOT
from ..uploads.s3 import download_file_from_s3
from .output import make_output_path
from .processing import run_ocr

# Download the source document file from S3
def source_s3_key(user_id: str, document_id: str, source_filename: str) -> str:
    return f"{user_id}/{document_id}/source/{source_filename}"


def resolve_input_file(source: dict, user_id: str, document_id: str):
    s3_bucket = source["s3_bucket"]
    source_filename = source["source_filename"]
    s3_key = source_s3_key(user_id, document_id, source_filename)
    local_path = (
        INPUT_ROOT
        / "sources"
        / user_id
        / document_id
        / source_filename
    )
    return download_file_from_s3(s3_bucket, s3_key, local_path)


def process_ocr(
    source: dict,
    user_id: str,
    document_id: str,
):
    # download the file to a local path
    input_file_path = resolve_input_file(source, user_id, document_id)
    # prepare output directory
    output_dir_path = make_output_path(user_id, document_id)

    # check file size to avoid processing very small / corrupted files
    if input_file_path.stat().st_size < 1000:
        raise ValueError(f"File too small / corrupted: {input_file_path}")

    # run ocr and get markdown output
    try:
        final_output = run_ocr(input_file_path)
    except Exception:
        print(f"OCR failed for {input_file_path}")
        traceback.print_exc()
        raise

    # write to output.md in the output directory
    with open(output_dir_path / "output.md", "w", encoding="utf-8") as f:
        f.write(final_output)

    return {
        "status": "ocr_completed",
        "user_id": user_id,
        "document_id": document_id,
        "source_filename": source["source_filename"],
        "local_markdown_path": str(output_dir_path / "output.md"),
    }
