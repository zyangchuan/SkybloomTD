from pathlib import Path

from .s3 import upload_output_to_s3

def upload_ocr_output(ocr_result: dict):
    if ocr_result.get("status") != "ocr_completed":
        return {
            "status": "skipped",
            "reason": "OCR did not complete successfully",
            "ocr_result": ocr_result,
        }

    user_id = ocr_result["user_id"]
    document_id = ocr_result["document_id"]
    local_markdown_path = Path(ocr_result["local_markdown_path"])
    document_output_dir = local_markdown_path.parent

    s3_result = upload_output_to_s3(document_output_dir, user_id, document_id)

    return {
        **ocr_result,
        **s3_result,
    }
