package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/database"
	"skybloom/game-service/internal/gamesession"
	"skybloom/game-service/internal/generation"
	"skybloom/game-service/internal/generationstatus"
	"skybloom/game-service/internal/mapcache"
	"skybloom/game-service/internal/messaging"
	"skybloom/game-service/internal/quizcache"
	"skybloom/game-service/internal/repository"
	"skybloom/game-service/internal/server"
)

func main() {
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
	if err := database.Migrate(ctx, db); err != nil {
		log.Fatalf("database migration error: %v", err)
	}

	levelRepo := repository.NewLevelRepository(db)
	statusStore, err := generationstatus.New(cfg.StatusRedisURL, cfg.GenerationStatusTTL)
	if err != nil {
		log.Fatalf("redis generation status configuration error: %v", err)
	}
	if statusStore != nil {
		defer statusStore.Close()
	}

	mapStore, err := mapcache.New(cfg.MapRedisURL, cfg.LevelMapCacheTTL)
	if err != nil {
		log.Fatalf("redis map cache configuration error: %v", err)
	}
	if mapStore != nil {
		defer mapStore.Close()
	}

	quizStore, err := quizcache.New(cfg.MapRedisURL, cfg.LevelQuizCacheTTL)
	if err != nil {
		log.Fatalf("redis quiz cache configuration error: %v", err)
	}
	if quizStore != nil {
		defer quizStore.Close()
	}

	sessionStore, err := gamesession.New(cfg.GameSessionRedisURL, cfg.GameSessionTTL)
	if err != nil {
		log.Fatalf("redis game session configuration error: %v", err)
	}
	if sessionStore != nil {
		defer sessionStore.Close()
	}

	publisher, err := messaging.NewPublisher(cfg.RabbitMQURL, cfg.Queue)
	if err != nil {
		log.Fatalf("rabbitmq publisher configuration error: %v", err)
	}
	defer publisher.Close()

	generationService := generation.NewService(cfg, levelRepo, statusStore, publisher)
	router := server.NewWithGenerationCachesSessionsAndRefills(cfg, levelRepo, mapStore, quizStore, levelRepo, generationService, statusStore, sessionStore, publisher).Router()
	addr := ":" + cfg.Port
	log.Printf("game-service listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}
