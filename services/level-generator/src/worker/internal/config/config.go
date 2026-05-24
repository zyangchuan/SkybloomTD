package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	RabbitMQURL                string
	Queue                      string
	DatabaseURL                string
	OpenAIAPIKey               string
	OpenAIBaseURL              string
	Model                      string
	Temperature                float64
	Timeout                    time.Duration
	MaxRetries                 int
	LevelSourceMaxChars        int32
	DocumentContentGRPCAddr    string
	DocumentContentGRPCTimeout time.Duration
}

func Load() (Config, error) {
	databaseURL, err := databaseURLFromEnv()
	if err != nil {
		return Config{}, err
	}
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return Config{}, errors.New("OPENAI_API_KEY is required")
	}
	return Config{
		RabbitMQURL:                envOrDefault("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/"),
		Queue:                      envOrDefault("LEVEL_GENERATOR_QUEUE", "level.generate"),
		DatabaseURL:                databaseURL,
		OpenAIAPIKey:               apiKey,
		OpenAIBaseURL:              strings.TrimRight(envOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"), "/"),
		Model:                      envOrDefault("LEVEL_LLM_MODEL", "gpt-4o-mini"),
		Temperature:                envFloat("LEVEL_LLM_TEMPERATURE", 0.2),
		Timeout:                    time.Duration(envFloat("LEVEL_LLM_TIMEOUT_SECONDS", 60)) * time.Second,
		MaxRetries:                 envInt("LEVEL_LLM_MAX_RETRIES", 1),
		LevelSourceMaxChars:        int32(envInt("LEVEL_SOURCE_MAX_CHARS", 24000)),
		DocumentContentGRPCAddr:    envOrDefault("DOCUMENT_CONTENT_GRPC_ADDR", envOrDefault("OCR_CONTENT_GRPC_ADDR", "localhost:50051")),
		DocumentContentGRPCTimeout: time.Duration(envFloat("DOCUMENT_CONTENT_GRPC_TIMEOUT_SECONDS", envFloat("OCR_CONTENT_GRPC_TIMEOUT_SECONDS", 30))) * time.Second,
	}, nil
}

func databaseURLFromEnv() (string, error) {
	if raw := strings.TrimSpace(firstNonEmpty(os.Getenv("LEVEL_DATABASE_URL"), os.Getenv("DATABASE_URL"), os.Getenv("POSTGRES_DSN"))); raw != "" {
		return normalizePostgresURL(raw), nil
	}

	host := firstNonEmpty(os.Getenv("LEVEL_POSTGRES_HOST"), os.Getenv("POSTGRES_HOST"), os.Getenv("AWS_RDS_POSTGRES_HOST"))
	port := firstNonEmpty(os.Getenv("LEVEL_POSTGRES_PORT"), os.Getenv("POSTGRES_PORT"), "5432")
	dbName := firstNonEmpty(os.Getenv("LEVEL_POSTGRES_DB"), os.Getenv("POSTGRES_DB"))
	user := firstNonEmpty(os.Getenv("LEVEL_POSTGRES_USER"), os.Getenv("POSTGRES_USER"))
	password := firstNonEmpty(os.Getenv("LEVEL_POSTGRES_PASSWORD"), os.Getenv("POSTGRES_PASSWORD"))
	sslMode := firstNonEmpty(os.Getenv("LEVEL_POSTGRES_SSLMODE"), os.Getenv("POSTGRES_SSLMODE"), "require")
	if host == "" || dbName == "" || user == "" || password == "" {
		return "", errors.New("set DATABASE_URL or POSTGRES_HOST, POSTGRES_DB, POSTGRES_USER, and POSTGRES_PASSWORD")
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + dbName,
	}
	query := u.Query()
	if sslMode != "" {
		query.Set("sslmode", sslMode)
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func normalizePostgresURL(raw string) string {
	if strings.HasPrefix(raw, "postgresql+psycopg://") {
		return "postgres://" + strings.TrimPrefix(raw, "postgresql+psycopg://")
	}
	if strings.HasPrefix(raw, "postgresql://") {
		return "postgres://" + strings.TrimPrefix(raw, "postgresql://")
	}
	return raw
}

func envOrDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func envFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return value
}
