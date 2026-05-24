package main

import (
	"log"
	"net/http"

	"skybloom/document-content-api/internal/api"
	"skybloom/document-content-api/internal/config"
	"skybloom/document-content-api/internal/messaging"
	"skybloom/document-content-api/internal/storage"
)

func main() {
	cfg := config.Load()

	publisher, err := messaging.NewPublisher(cfg.RabbitMQURL, cfg.DocumentContentQueue)
	if err != nil {
		log.Fatalf("rabbitmq connection error: %v", err)
	}
	defer publisher.Close()

	storageClient, err := storage.NewFromEnv()
	if err != nil {
		log.Fatalf("storage configuration error: %v", err)
	}

	router := api.NewRouter(cfg, publisher, storageClient)

	addr := ":" + cfg.Port
	log.Printf("document-content-api listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
