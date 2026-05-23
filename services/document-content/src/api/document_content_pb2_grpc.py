import grpc

try:
    from . import document_content_pb2 as document__content__pb2
except ImportError:
    import document_content_pb2 as document__content__pb2


class DocumentContentServiceStub:
    def __init__(self, channel):
        self.GetSubChapter = channel.unary_unary(
            "/document_content.v1.DocumentContentService/GetSubChapter",
            request_serializer=document__content__pb2.GetSubChapterRequest.SerializeToString,
            response_deserializer=document__content__pb2.GetSubChapterResponse.FromString,
        )


class DocumentContentServiceServicer:
    def GetSubChapter(self, request, context):
        context.set_code(grpc.StatusCode.UNIMPLEMENTED)
        context.set_details("Method not implemented")
        raise NotImplementedError("Method not implemented")


def add_DocumentContentServiceServicer_to_server(servicer, server):
    rpc_method_handlers = {
        "GetSubChapter": grpc.unary_unary_rpc_method_handler(
            servicer.GetSubChapter,
            request_deserializer=document__content__pb2.GetSubChapterRequest.FromString,
            response_serializer=document__content__pb2.GetSubChapterResponse.SerializeToString,
        ),
    }
    generic_handler = grpc.method_handlers_generic_handler(
        "document_content.v1.DocumentContentService",
        rpc_method_handlers,
    )
    server.add_generic_rpc_handlers((generic_handler,))
