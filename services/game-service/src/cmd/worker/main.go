package main

import (
	"context"
	"log"
	"time"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/database"
	"skybloom/game-service/internal/generationstatus"
	"skybloom/game-service/internal/generator"
	"skybloom/game-service/internal/mapcache"
	"skybloom/game-service/internal/quizcache"
	"skybloom/game-service/internal/repository"
	"skybloom/game-service/internal/source"
	workerpkg "skybloom/game-service/internal/worker"
)

func main() {
	source.RegisterJSONCodec()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}
	if cfg.OpenAIAPIKey == "" {
		log.Fatal("configuration error: OPENAI_API_KEY is required")
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

	statusStore, err := generationstatus.New(cfg.StatusRedisURL, cfg.GenerationStatusTTL)
	if err != nil {
		log.Fatalf("redis status configuration error: %v", err)
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

	levelRepo := repository.NewLevelRepository(db)
	sourceClient := source.NewClient(cfg.DocumentContentGRPCAddr, cfg.DocumentContentGRPCTimeout, cfg.LevelSourceMaxChars)
	generatorClient := generator.NewClient(generator.Config{
		APIKey:      cfg.OpenAIAPIKey,
		BaseURL:     cfg.OpenAIBaseURL,
		Model:       cfg.Model,
		Temperature: cfg.Temperature,
		Timeout:     cfg.Timeout,
		MaxRetries:  cfg.MaxRetries,
	})

	worker := workerpkg.NewWithStoresAndQuizCache(cfg, levelRepo, sourceClient, generatorClient, statusStore, mapStore, quizStore)
	if err := worker.Consume(context.Background()); err != nil {
		log.Fatalf("worker error: %v", err)
	}
}
