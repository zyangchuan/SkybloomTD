from concurrent import futures
import logging

import grpc

import document_content_pb2
import document_content_pb2_grpc
from config import CONTENT_GRPC_HOST, CONTENT_GRPC_PORT
from content_repository import (
    ContentNotFoundError,
    ContentRequestError,
    ContentUnavailableError,
    fetch_sub_chapter_content,
)


class DocumentContentServicer(document_content_pb2_grpc.DocumentContentServiceServicer):
    def GetSubChapter(self, request, context):
        try:
            sub_chapter = fetch_sub_chapter_content(
                user_id=request.user_id,
                sub_chapter_id=request.sub_chapter_id,
                max_chars=request.max_chars,
            )
        except ContentRequestError as exc:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, str(exc))
        except ContentNotFoundError as exc:
            context.abort(grpc.StatusCode.NOT_FOUND, str(exc))
        except ContentUnavailableError as exc:
            context.abort(grpc.StatusCode.FAILED_PRECONDITION, str(exc))

        return document_content_pb2.GetSubChapterResponse(
            sub_chapter=document_content_pb2.SubChapterContent(
                normalized_user_id=sub_chapter["normalized_user_id"],
                requested_user_id=sub_chapter["requested_user_id"],
                sub_chapter_id=sub_chapter["sub_chapter_id"],
                document_id=sub_chapter["document_id"],
                chapter_id=sub_chapter["chapter_id"],
                sub_chapter_index=sub_chapter["sub_chapter_index"],
                title=sub_chapter["title"],
                start_line=sub_chapter["start_line"],
                end_line=sub_chapter["end_line"],
                source_text=sub_chapter["source_text"],
                source_chunk_ids=sub_chapter["source_chunk_ids"],
                chunk_count=sub_chapter["chunk_count"],
                candidate_chunk_count=sub_chapter["candidate_chunk_count"],
                chunk_lookup_strategy=sub_chapter["chunk_lookup_strategy"],
                source_char_count=sub_chapter["source_char_count"],
                source_truncated=sub_chapter["source_truncated"],
                markdown_cache_hit=sub_chapter["markdown_cache_hit"],
                markdown_cache_key=sub_chapter["markdown_cache_key"],
                source_content_hash=sub_chapter["source_content_hash"],
            )
        )


def serve() -> None:
    logging.basicConfig(level=logging.INFO)
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    document_content_pb2_grpc.add_DocumentContentServiceServicer_to_server(
        DocumentContentServicer(),
        server,
    )
    address = f"{CONTENT_GRPC_HOST}:{CONTENT_GRPC_PORT}"
    server.add_insecure_port(address)
    server.start()
    logging.info("Document content gRPC server listening on %s", address)
    server.wait_for_termination()


if __name__ == "__main__":
    serve()
