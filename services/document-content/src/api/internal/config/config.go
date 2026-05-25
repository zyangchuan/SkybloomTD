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
	Port                 string
	DatabaseURL          string
	RabbitMQURL          string
	DocumentContentQueue string
	TempDir              string
	RedisURL             string
	TaskStatusTTL        time.Duration
}

func Load() (Config, error) {
	databaseURL, err := databaseURLFromEnv()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Port:                 envOrDefault("DOCUMENT_CONTENT_API_PORT", envOrDefault("PORT", "8000")),
		DatabaseURL:          databaseURL,
		RabbitMQURL:          envOrDefault("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/"),
		DocumentContentQueue: envOrDefault("DOCUMENT_CONTENT_QUEUE", "document.process"),
		TempDir:              envOrDefault("TEMP_DIR", "/temp"),
		RedisURL:             envOrDefault("REDIS_URL", "redis://redis:6379/0"),
		TaskStatusTTL:        envDurationSeconds("TASK_STATUS_TTL_SECONDS", 7*24*time.Hour),
	}, nil
}

func databaseURLFromEnv() (string, error) {
	if raw := strings.TrimSpace(firstNonEmpty(
		os.Getenv("DOCUMENT_CONTENT_DATABASE_URL"),
		os.Getenv("DATABASE_URL"),
		os.Getenv("POSTGRES_DSN"),
	)); raw != "" {
		return normalizePostgresURL(raw), nil
	}

	host := firstNonEmpty(os.Getenv("DOCUMENT_CONTENT_POSTGRES_HOST"), os.Getenv("POSTGRES_HOST"), os.Getenv("AWS_RDS_POSTGRES_HOST"))
	port := firstNonEmpty(os.Getenv("DOCUMENT_CONTENT_POSTGRES_PORT"), os.Getenv("POSTGRES_PORT"), "5432")
	dbName := firstNonEmpty(os.Getenv("DOCUMENT_CONTENT_POSTGRES_DB"), os.Getenv("POSTGRES_DB"))
	user := firstNonEmpty(os.Getenv("DOCUMENT_CONTENT_POSTGRES_USER"), os.Getenv("POSTGRES_USER"))
	password := firstNonEmpty(os.Getenv("DOCUMENT_CONTENT_POSTGRES_PASSWORD"), os.Getenv("POSTGRES_PASSWORD"))
	sslMode := firstNonEmpty(os.Getenv("DOCUMENT_CONTENT_POSTGRES_SSLMODE"), os.Getenv("POSTGRES_SSLMODE"), "require")
	if strings.TrimSpace(host) == "" || strings.TrimSpace(dbName) == "" ||
		strings.TrimSpace(user) == "" || password == "" {
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

func envDurationSeconds(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
