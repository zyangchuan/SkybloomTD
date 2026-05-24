package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"

	"skybloom/document-content-api/internal/api"
	"skybloom/document-content-api/internal/config"
	"skybloom/document-content-api/internal/messaging"
	"skybloom/document-content-api/internal/storage"
)

func main() {
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialise environment variables
	cfg := config.Load()

	// RabbitMQ publisher
	publisher, err := messaging.NewPublisher(cfg.RabbitMQURL, cfg.DocumentContentQueue)
	if err != nil {
		log.Fatalf("rabbitmq connection error: %v", err)
	}
	defer publisher.Close()

	// Supabase S3 bucket client
	storageClient, err := storage.NewFromEnv()
	if err != nil {
		log.Fatalf("storage configuration error: %v", err)
	}

	// HTTP server router
	router := api.NewRouter(cfg, publisher, storageClient)

	addr := ":" + cfg.Port
	log.Printf("document-content-api listening on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
