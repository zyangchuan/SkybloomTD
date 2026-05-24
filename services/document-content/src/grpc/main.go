package main

import (
	"context"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"

	"skybloom/document-content-grpc/internal/codec"
	"skybloom/document-content-grpc/internal/config"
	"skybloom/document-content-grpc/internal/database"
	"skybloom/document-content-grpc/internal/grpcserver"
	"skybloom/document-content-grpc/internal/repository"
	"skybloom/document-content-grpc/internal/service"
	"skybloom/document-content-grpc/internal/storage"
)

func main() {
	encoding.RegisterCodec(codec.JSONCodec{})

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, closeDB, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection error: %v", err)
	}
	defer closeDB()

	loader, err := storage.NewMarkdownLoader()
	if err != nil {
		log.Fatalf("storage configuration error: %v", err)
	}

	listener, err := net.Listen("tcp", net.JoinHostPort(cfg.Host, cfg.Port))
	if err != nil {
		log.Fatalf("listen error: %v", err)
	}

	documents := repository.NewDocumentRepository(db)
	grpcServer := grpc.NewServer()
	grpcserver.RegisterDocumentContentService(grpcServer, service.NewServer(documents, loader))

	log.Printf("document-content-grpc listening on %s", listener.Addr())
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("grpc server error: %v", err)
	}
}
