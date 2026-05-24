package grpcserver

import (
	"context"

	"google.golang.org/grpc"

	"skybloom/document-content-grpc/internal/models"
)

type DocumentContentServiceServer interface {
	GetSubChapter(context.Context, *models.GetSubChapterRequest) (*models.GetSubChapterResponse, error)
}

func RegisterDocumentContentService(server *grpc.Server, service DocumentContentServiceServer) {
	server.RegisterService(&grpc.ServiceDesc{
		ServiceName: "document_content.v1.DocumentContentService",
		HandlerType: (*DocumentContentServiceServer)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "GetSubChapter",
				Handler:    getSubChapterHandler,
			},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "document_content.proto",
	}, service)
}

func getSubChapterHandler(service any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	request := new(models.GetSubChapterRequest)
	if err := decode(request); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return service.(DocumentContentServiceServer).GetSubChapter(ctx, request)
	}
	info := &grpc.UnaryServerInfo{
		Server:     service,
		FullMethod: "/document_content.v1.DocumentContentService/GetSubChapter",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return service.(DocumentContentServiceServer).GetSubChapter(ctx, req.(*models.GetSubChapterRequest))
	}
	return interceptor(ctx, request, info, handler)
}
