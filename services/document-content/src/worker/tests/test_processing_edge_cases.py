import sys
import types
import unittest

# ---------------------------------------------------------------------------
# Stub out heavy dependencies so the module can be imported without real infra
# ---------------------------------------------------------------------------

sys.modules.setdefault("pika", types.ModuleType("pika"))

redis_stub = types.ModuleType("redis")


class _FakeRedis:
    @classmethod
    def from_url(cls, *args, **kwargs):
        return cls()

    def set(self, *args, **kwargs):
        return None


redis_stub.Redis = _FakeRedis
sys.modules.setdefault("redis", redis_stub)

from app.rabbitmq_worker import process_job  # noqa: E402


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _job(**overrides):
    base = {
        "task_id": "task-1",
        "source": "/tmp/input.pdf",
        "user_id": "user-1",
        "document_id": "document-1",
        "filename": "input.pdf",
    }
    base.update(overrides)
    return base


def _record(statuses):
    def recorder(task_id, document_id, status, error=None):
        statuses.append((task_id, document_id, status, error))

    return recorder


# ---------------------------------------------------------------------------
# Upload-step failure
# ---------------------------------------------------------------------------


class TestUploadStepFailure(unittest.TestCase):
    def test_marks_failed_when_upload_step_raises(self):
        statuses = []

        def fail_upload(ocr_result):
            raise RuntimeError("S3 unavailable")

        with self.assertRaisesRegex(RuntimeError, "S3 unavailable"):
            process_job(
                _job(),
                process_ocr_fn=lambda *a, **kw: {"status": "ocr_completed"},
                upload_ocr_output_fn=fail_upload,
                index_ocr_output_fn=lambda r: r,
                set_task_status_fn=_record(statuses),
            )

        self.assertEqual(
            [
                ("task-1", "document-1", "processing", None),
                ("task-1", "document-1", "failed", "S3 unavailable"),
            ],
            statuses,
        )

    def test_marks_failed_when_index_step_raises(self):
        statuses = []

        def fail_index(upload_result):
            raise RuntimeError("Index service down")

        with self.assertRaisesRegex(RuntimeError, "Index service down"):
            process_job(
                _job(),
                process_ocr_fn=lambda *a, **kw: {"status": "ocr_completed"},
                upload_ocr_output_fn=lambda r: {"status": "uploaded"},
                index_ocr_output_fn=fail_index,
                set_task_status_fn=_record(statuses),
            )

        self.assertEqual(
            [
                ("task-1", "document-1", "processing", None),
                ("task-1", "document-1", "failed", "Index service down"),
            ],
            statuses,
        )


# ---------------------------------------------------------------------------
# Status recording order
# ---------------------------------------------------------------------------


class TestStatusRecordingOrder(unittest.TestCase):
    def test_processing_status_recorded_before_any_work_starts(self):
        """The first status update must be 'processing' before OCR begins."""
        work_started = []
        statuses = []

        def ocr_that_tracks_call_order(source, user_id, document_id, filename):
            work_started.append(len(statuses))  # how many statuses when OCR began
            return {"status": "ocr_completed"}

        process_job(
            _job(),
            process_ocr_fn=ocr_that_tracks_call_order,
            upload_ocr_output_fn=lambda r: {"status": "uploaded"},
            index_ocr_output_fn=lambda r: {"status": "indexed"},
            set_task_status_fn=_record(statuses),
        )

        self.assertEqual(1, work_started[0], "processing status must be recorded before OCR begins")

    def test_successful_status_recorded_after_all_steps_complete(self):
        """The 'successful' status must only appear after indexing finishes."""
        index_calls = []
        statuses = []

        def index_that_tracks_call_order(upload_result):
            index_calls.append(len(statuses))
            return {"status": "indexed"}

        process_job(
            _job(),
            process_ocr_fn=lambda *a, **kw: {"status": "ocr_completed"},
            upload_ocr_output_fn=lambda r: {"status": "uploaded"},
            index_ocr_output_fn=index_that_tracks_call_order,
            set_task_status_fn=_record(statuses),
        )

        # indexing ran when only 'processing' status existed (count=1)
        self.assertEqual(1, index_calls[0])
        # final status is 'successful'
        self.assertEqual("successful", statuses[-1][2])


# ---------------------------------------------------------------------------
# Error message propagation
# ---------------------------------------------------------------------------


class TestErrorMessagePropagation(unittest.TestCase):
    def test_error_message_is_preserved_verbatim_from_exception(self):
        statuses = []
        detailed_message = "OCR failed: page 3 could not be parsed (codec error)"

        with self.assertRaises(RuntimeError):
            process_job(
                _job(),
                process_ocr_fn=lambda *a, **kw: (_ for _ in ()).throw(
                    RuntimeError(detailed_message)
                ),
                upload_ocr_output_fn=lambda r: r,
                index_ocr_output_fn=lambda r: r,
                set_task_status_fn=_record(statuses),
            )

        failed_status = next(s for s in statuses if s[2] == "failed")
        self.assertEqual(detailed_message, failed_status[3])

    def test_error_message_from_skipped_indexing_is_reason_field(self):
        """When indexing reports it was skipped, the reason becomes the error message."""
        statuses = []
        reason = "OCR output was not uploaded to S3"

        process_job(
            _job(),
            process_ocr_fn=lambda *a, **kw: {"status": "ocr_completed"},
            upload_ocr_output_fn=lambda r: {"status": "skipped"},
            index_ocr_output_fn=lambda r: {"status": "skipped", "reason": reason},
            set_task_status_fn=_record(statuses),
        )

        failed_status = next(s for s in statuses if s[2] == "failed")
        self.assertEqual(reason, failed_status[3])


# ---------------------------------------------------------------------------
# Job field forwarding
# ---------------------------------------------------------------------------


class TestJobFieldForwarding(unittest.TestCase):
    def test_ocr_fn_receives_all_job_fields(self):
        """process_job must forward source, user_id, document_id, filename to OCR."""
        received = {}

        def capture_ocr(source, user_id, document_id, filename):
            received.update(
                source=source,
                user_id=user_id,
                document_id=document_id,
                filename=filename,
            )
            return {"status": "ocr_completed"}

        process_job(
            _job(
                source="/data/upload.pdf",
                user_id="user-42",
                document_id="doc-99",
                filename="lecture.pdf",
            ),
            process_ocr_fn=capture_ocr,
            upload_ocr_output_fn=lambda r: {"status": "uploaded"},
            index_ocr_output_fn=lambda r: {"status": "indexed"},
            set_task_status_fn=_record([]),
        )

        self.assertEqual("/data/upload.pdf", received["source"])
        self.assertEqual("user-42", received["user_id"])
        self.assertEqual("doc-99", received["document_id"])
        self.assertEqual("lecture.pdf", received["filename"])

    def test_set_task_status_receives_correct_task_and_document_ids(self):
        statuses = []

        process_job(
            _job(task_id="task-xyz", document_id="doc-abc"),
            process_ocr_fn=lambda *a, **kw: {"status": "ocr_completed"},
            upload_ocr_output_fn=lambda r: {"status": "uploaded"},
            index_ocr_output_fn=lambda r: {"status": "indexed"},
            set_task_status_fn=_record(statuses),
        )

        for task_id, document_id, _, _ in statuses:
            self.assertEqual("task-xyz", task_id)
            self.assertEqual("doc-abc", document_id)


if __name__ == "__main__":
    unittest.main()
