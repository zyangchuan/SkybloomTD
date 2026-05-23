from google.protobuf import descriptor_pb2 as _descriptor_pb2
from google.protobuf import descriptor_pool as _descriptor_pool
from google.protobuf import message_factory as _message_factory
from google.protobuf import symbol_database as _symbol_database

_sym_db = _symbol_database.Default()


def _add_field(message, name, number, field_type, label=1, type_name=None):
    field = message.field.add()
    field.name = name
    field.number = number
    field.label = label
    field.type = field_type
    if type_name:
        field.type_name = type_name


def _build_file_descriptor_proto():
    file_descriptor = _descriptor_pb2.FileDescriptorProto()
    file_descriptor.name = "document_content.proto"
    file_descriptor.package = "document_content.v1"
    file_descriptor.syntax = "proto3"

    request = file_descriptor.message_type.add()
    request.name = "GetSubChapterRequest"
    _add_field(request, "user_id", 1, 9)
    _add_field(request, "sub_chapter_id", 2, 9)
    _add_field(request, "max_chars", 3, 5)

    sub_chapter = file_descriptor.message_type.add()
    sub_chapter.name = "SubChapterContent"
    _add_field(sub_chapter, "normalized_user_id", 1, 9)
    _add_field(sub_chapter, "requested_user_id", 2, 9)
    _add_field(sub_chapter, "sub_chapter_id", 3, 9)
    _add_field(sub_chapter, "document_id", 4, 9)
    _add_field(sub_chapter, "chapter_id", 5, 9)
    _add_field(sub_chapter, "sub_chapter_index", 6, 5)
    _add_field(sub_chapter, "title", 7, 9)
    _add_field(sub_chapter, "start_line", 8, 5)
    _add_field(sub_chapter, "end_line", 9, 5)
    _add_field(sub_chapter, "source_text", 10, 9)
    _add_field(sub_chapter, "source_chunk_ids", 11, 9, label=3)
    _add_field(sub_chapter, "chunk_count", 12, 5)
    _add_field(sub_chapter, "candidate_chunk_count", 13, 5)
    _add_field(sub_chapter, "chunk_lookup_strategy", 14, 9)
    _add_field(sub_chapter, "source_char_count", 15, 5)
    _add_field(sub_chapter, "source_truncated", 16, 8)
    _add_field(sub_chapter, "markdown_cache_hit", 17, 8)
    _add_field(sub_chapter, "markdown_cache_key", 18, 9)
    _add_field(sub_chapter, "source_content_hash", 19, 9)

    response = file_descriptor.message_type.add()
    response.name = "GetSubChapterResponse"
    _add_field(
        response,
        "sub_chapter",
        1,
        11,
        type_name=".document_content.v1.SubChapterContent",
    )

    service = file_descriptor.service.add()
    service.name = "DocumentContentService"
    method = service.method.add()
    method.name = "GetSubChapter"
    method.input_type = ".document_content.v1.GetSubChapterRequest"
    method.output_type = ".document_content.v1.GetSubChapterResponse"

    return file_descriptor


try:
    DESCRIPTOR = _descriptor_pool.Default().AddSerializedFile(
        _build_file_descriptor_proto().SerializeToString()
    )
except Exception:
    DESCRIPTOR = _descriptor_pool.Default().FindFileByName("document_content.proto")


def _message_class(name):
    descriptor = DESCRIPTOR.message_types_by_name[name]
    if hasattr(_message_factory, "GetMessageClass"):
        message_cls = _message_factory.GetMessageClass(descriptor)
    else:
        message_cls = _message_factory.MessageFactory().GetPrototype(descriptor)
    message_cls.__module__ = __name__
    _sym_db.RegisterMessage(message_cls)
    return message_cls


GetSubChapterRequest = _message_class("GetSubChapterRequest")
SubChapterContent = _message_class("SubChapterContent")
GetSubChapterResponse = _message_class("GetSubChapterResponse")
