package main

import (
	"log"
	"net/http"

	"skybloom/level-generator-api/internal/api"
	"skybloom/level-generator-api/internal/config"
	"skybloom/level-generator-api/internal/messaging"
)

func main() {
	cfg := config.Load()
	publisher, err := messaging.NewPublisher(cfg.RabbitMQURL, cfg.LevelGeneratorQueue)
	if err != nil {
		log.Fatalf("rabbitmq connection error: %v", err)
	}
	defer publisher.Close()

	router := api.NewRouter(publisher)

	addr := ":" + cfg.Port
	log.Printf("level-generator-api listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
