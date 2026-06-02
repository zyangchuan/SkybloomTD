import sys
import threading
import types
import unittest


class FakeRedisClient:
    @classmethod
    def from_url(cls, *args, **kwargs):
        return cls()

    def set(self, *args, **kwargs):
        return None


sys.modules.setdefault("pika", types.ModuleType("pika"))
redis_module = types.ModuleType("redis")
redis_module.Redis = FakeRedisClient
sys.modules.setdefault("redis", redis_module)

from app.rabbitmq_worker import process_job, run_with_heartbeat_pump


class RabbitMQWorkerTest(unittest.TestCase):
    def test_process_job_marks_processing_then_successful_after_indexing(self):
        statuses = []
        job = self.document_job()

        result = process_job(
            job,
            process_ocr_fn=lambda source, user_id, document_id, filename: {
                "status": "ocr_completed",
                "source": source,
                "user_id": user_id,
                "document_id": document_id,
                "filename": filename,
            },
            upload_ocr_output_fn=lambda ocr_result: {
                "status": "uploaded",
                "ocr_result": ocr_result,
            },
            index_ocr_output_fn=lambda upload_result: {
                "status": "indexed",
                "upload_result": upload_result,
            },
            set_task_status_fn=self.record_status(statuses),
        )

        self.assertEqual("indexed", result["status"])
        self.assertEqual(
            [
                ("task-1", "document-1", "processing", None),
                ("task-1", "document-1", "successful", None),
            ],
            statuses,
        )

    def test_process_job_marks_failed_on_exception(self):
        statuses = []
        job = self.document_job()

        def fail_ocr(*args, **kwargs):
            raise RuntimeError("OCR exploded")

        with self.assertRaisesRegex(RuntimeError, "OCR exploded"):
            process_job(
                job,
                process_ocr_fn=fail_ocr,
                upload_ocr_output_fn=lambda ocr_result: ocr_result,
                index_ocr_output_fn=lambda upload_result: upload_result,
                set_task_status_fn=self.record_status(statuses),
            )

        self.assertEqual(
            [
                ("task-1", "document-1", "processing", None),
                ("task-1", "document-1", "failed", "OCR exploded"),
            ],
            statuses,
        )

    def test_process_job_marks_failed_when_indexing_does_not_complete(self):
        statuses = []
        job = self.document_job()

        result = process_job(
            job,
            process_ocr_fn=lambda *args: {"status": "ocr_completed"},
            upload_ocr_output_fn=lambda ocr_result: {"status": "skipped"},
            index_ocr_output_fn=lambda upload_result: {
                "status": "skipped",
                "reason": "OCR output was not uploaded to S3",
            },
            set_task_status_fn=self.record_status(statuses),
        )

        self.assertEqual("skipped", result["status"])
        self.assertEqual(
            [
                ("task-1", "document-1", "processing", None),
                (
                    "task-1",
                    "document-1",
                    "failed",
                    "OCR output was not uploaded to S3",
                ),
            ],
            statuses,
        )

    def test_run_with_heartbeat_pump_processes_connection_events_during_work(self):
        release_work = threading.Event()
        pump_calls = []

        class FakeConnection:
            def process_data_events(self, time_limit):
                pump_calls.append(time_limit)
                release_work.set()

        def slow_work():
            release_work.wait(timeout=1)
            return {"status": "indexed"}

        result = run_with_heartbeat_pump(
            FakeConnection(),
            slow_work,
            poll_seconds=0.1,
        )

        self.assertEqual({"status": "indexed"}, result)
        self.assertGreaterEqual(len(pump_calls), 1)

    @staticmethod
    def document_job():
        return {
            "task_id": "task-1",
            "source": "/tmp/input.pdf",
            "user_id": "user-1",
            "document_id": "document-1",
            "filename": "input.pdf",
        }

    @staticmethod
    def record_status(statuses):
        def recorder(task_id, document_id, status, error=None):
            statuses.append((task_id, document_id, status, error))

        return recorder


if __name__ == "__main__":
    unittest.main()
