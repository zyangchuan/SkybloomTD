package main

import (
	"context"
	"log"
	"os"

	"github.com/gin-gonic/gin"

	"skybloom/document-content-api/internal/api"
	"skybloom/document-content-api/internal/config"
	"skybloom/document-content-api/internal/database"
	"skybloom/document-content-api/internal/messaging"
	"skybloom/document-content-api/internal/repository"
	"skybloom/document-content-api/internal/storage"
	"skybloom/document-content-api/internal/taskstatus"
)

func main() {
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialise environment variables
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	ctx := context.Background()

	// Postgres document repository
	db, closeDB, err := database.Open(ctx, cfg.DatabaseURL)

	if err != nil {
		log.Fatalf("database connection error: %v", err)
	}
	
	if err := database.Migrate(ctx, db); err != nil {
		log.Fatalf("database migration error: %v", err)
	}

	defer closeDB()

	documents := repository.NewDocumentRepository(db)

	// RabbitMQ publisher
	publisher, err := messaging.NewPublisher(cfg.RabbitMQURL, cfg.DocumentContentQueue)
	if err != nil {
		log.Fatalf("rabbitmq connection error: %v", err)
	}
	defer publisher.Close()

	// Supabase S3 bucket client
	storageClient, err := storage.NewFromConfig(cfg)
	if err != nil {
		log.Fatalf("storage configuration error: %v", err)
	}

	// Redis task status store
	taskStatusStore, err := taskstatus.New(cfg.RedisURL, cfg.TaskStatusTTL)
	if err != nil {
		log.Fatalf("redis configuration error: %v", err)
	}
	defer taskStatusStore.Close()

	// HTTP server router
	router := api.NewRouter(cfg, publisher, storageClient, documents, taskStatusStore)

	addr := ":" + cfg.Port
	log.Printf("document-content-api listening on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
