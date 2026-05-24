package config

import (
	"os"
	"strings"
)

type Config struct {
	Port                string
	RabbitMQURL         string
	LevelGeneratorQueue string
}

func Load() Config {
	return Config{
		Port:                envOrDefault("LEVEL_GENERATOR_API_PORT", envOrDefault("PORT", "8000")),
		RabbitMQURL:         envOrDefault("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/"),
		LevelGeneratorQueue: envOrDefault("LEVEL_GENERATOR_QUEUE", "level.generate"),
	}
}

func envOrDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
