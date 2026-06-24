__all__ = ["index_ocr_output"]


def __getattr__(name):
    if name == "index_ocr_output":
        from .tasks import index_ocr_output

        return index_ocr_output
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
