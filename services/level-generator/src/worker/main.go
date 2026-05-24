package main

import (
	"context"
	"log"
	"time"

	"skybloom/level-generator-worker/internal/config"
	"skybloom/level-generator-worker/internal/database"
	"skybloom/level-generator-worker/internal/generator"
	"skybloom/level-generator-worker/internal/repository"
	"skybloom/level-generator-worker/internal/source"
	workerpkg "skybloom/level-generator-worker/internal/worker"
)

func main() {
	source.RegisterJSONCodec()

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
	sourceClient := source.NewClient(cfg.DocumentContentGRPCAddr, cfg.DocumentContentGRPCTimeout, cfg.LevelSourceMaxChars)
	generatorClient := generator.NewClient(generator.Config{
		APIKey:      cfg.OpenAIAPIKey,
		BaseURL:     cfg.OpenAIBaseURL,
		Model:       cfg.Model,
		Temperature: cfg.Temperature,
		Timeout:     cfg.Timeout,
		MaxRetries:  cfg.MaxRetries,
	})

	worker := workerpkg.New(cfg, levelRepo, sourceClient, generatorClient)
	if err := worker.Consume(context.Background()); err != nil {
		log.Fatalf("worker error: %v", err)
	}
}
