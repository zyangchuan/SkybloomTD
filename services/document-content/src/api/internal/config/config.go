package config

import (
	"os"
	"strings"
)

type Config struct {
	Port                 string
	RabbitMQURL          string
	DocumentContentQueue string
	TempDir              string
}

func Load() Config {
	return Config{
		Port:                 envOrDefault("DOCUMENT_CONTENT_API_PORT", envOrDefault("PORT", "8000")),
		RabbitMQURL:          envOrDefault("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/"),
		DocumentContentQueue: envOrDefault("DOCUMENT_CONTENT_QUEUE", "document.process"),
		TempDir:              envOrDefault("TEMP_DIR", "/temp"),
	}
}

func envOrDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
