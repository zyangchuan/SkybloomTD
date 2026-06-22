import traceback
from pathlib import Path
from typing import Any

from paddleocr import PPStructureV3

def create_pipeline() -> PPStructureV3:
    return PPStructureV3(
        device="gpu",
        lang="en",
        precision="fp16",
        layout_detection_model_name="PP-DocLayout-L",
        text_recognition_batch_size=32,
        text_recognition_model_name="en_PP-OCRv3_mobile_rec",
        formula_recognition_batch_size=16,
        formula_recognition_model_name="PP-FormulaNet_plus-S",
    )

pipeline = create_pipeline()

def run_ocr(file_path: Path) -> str:
    output = pipeline.predict(str(file_path))
    markdown_pages = collect_markdown_pages(output)
    markdown_texts = pipeline.concatenate_markdown_pages(markdown_pages)

    return markdown_texts.get("markdown_texts", "")


def collect_markdown_pages(output: Any) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    markdown_pages = []

    for idx, res in enumerate(output):
        try:
            md_info = res.markdown

            if md_info:
                markdown_pages.append(md_info)
        except Exception:
            print(f"Failed processing page {idx}")
            traceback.print_exc()

    return markdown_pages
