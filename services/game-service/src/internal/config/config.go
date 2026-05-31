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
	Port                       string
	RabbitMQURL                string
	Queue                      string
	DatabaseURL                string
	StatusRedisURL             string
	MapRedisURL                string
	GameSessionRedisURL        string
	GenerationStatusTTL        time.Duration
	LevelMapCacheTTL           time.Duration
	LevelQuizCacheTTL          time.Duration
	GameSessionTTL             time.Duration
	OpenAIAPIKey               string
	OpenAIBaseURL              string
	Model                      string
	Temperature                float64
	Timeout                    time.Duration
	MaxRetries                 int
	LevelSourceMaxChars        int32
	DocumentContentGRPCAddr    string
	DocumentContentGRPCTimeout time.Duration
	PublicAPIBasePath          string
	AllowedOrigins             []string
	AllowInsecureUserQueryAuth bool
}

func Load() (Config, error) {
	databaseURL, err := databaseURLFromEnv()
	if err != nil {
		return Config{}, err
	}

	generationStatusTTL := envDurationSeconds("GAME_GENERATION_STATUS_TTL_SECONDS", envDurationSeconds("LEVEL_GENERATION_STATUS_TTL_SECONDS", 24*time.Hour))
	levelMapCacheTTL := envDurationSeconds("LEVEL_MAP_CACHE_TTL_SECONDS", 24*time.Hour)
	mapRedisURL := envOrDefault("GAME_MAP_REDIS_URL", "redis://game-redis:6379/0")

	return Config{
		Port:                       envOrDefault("GAME_SERVICE_PORT", envOrDefault("PORT", "8081")),
		RabbitMQURL:                envOrDefault("RABBITMQ_URL", "amqp://guest:guest@rabbitmq:5672/"),
		Queue:                      envOrDefault("GAME_GENERATION_QUEUE", "game.generation.generate"),
		DatabaseURL:                databaseURL,
		StatusRedisURL:             envOrDefault("GAME_STATUS_REDIS_URL", envOrDefault("REDIS_URL", "redis://redis:6379/0")),
		MapRedisURL:                mapRedisURL,
		GameSessionRedisURL:        envOrDefault("GAME_SESSION_REDIS_URL", mapRedisURL),
		GenerationStatusTTL:        generationStatusTTL,
		LevelMapCacheTTL:           levelMapCacheTTL,
		LevelQuizCacheTTL:          envDurationSeconds("LEVEL_QUIZ_CACHE_TTL_SECONDS", levelMapCacheTTL),
		GameSessionTTL:             envDurationSeconds("GAME_SESSION_TTL_SECONDS", 2*time.Hour),
		OpenAIAPIKey:               strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		OpenAIBaseURL:              strings.TrimRight(envOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"), "/"),
		Model:                      envOrDefault("LEVEL_LLM_MODEL", "gpt-5.4-mini"),
		Temperature:                envFloat("LEVEL_LLM_TEMPERATURE", 0.2),
		Timeout:                    time.Duration(envFloat("LEVEL_LLM_TIMEOUT_SECONDS", 60)) * time.Second,
		MaxRetries:                 envInt("LEVEL_LLM_MAX_RETRIES", 3),
		LevelSourceMaxChars:        int32(envInt("LEVEL_SOURCE_MAX_CHARS", 24000)),
		DocumentContentGRPCAddr:    envOrDefault("DOCUMENT_CONTENT_GRPC_ADDR", envOrDefault("OCR_CONTENT_GRPC_ADDR", "document-content-grpc:50051")),
		DocumentContentGRPCTimeout: time.Duration(envFloat("DOCUMENT_CONTENT_GRPC_TIMEOUT_SECONDS", envFloat("OCR_CONTENT_GRPC_TIMEOUT_SECONDS", 30))) * time.Second,
		PublicAPIBasePath:          envOrDefault("GAME_SERVICE_PUBLIC_API_BASE_PATH", "/api/game-service"),
		AllowedOrigins:             envCSV("GAME_SERVICE_ALLOWED_ORIGINS"),
		AllowInsecureUserQueryAuth: envBool("GAME_SERVICE_ALLOW_INSECURE_USER_QUERY", false),
	}, nil
}

func databaseURLFromEnv() (string, error) {
	if raw := strings.TrimSpace(firstNonEmpty(os.Getenv("GAME_DATABASE_URL"), os.Getenv("LEVEL_DATABASE_URL"), os.Getenv("DATABASE_URL"), os.Getenv("POSTGRES_DSN"))); raw != "" {
		return normalizePostgresURL(raw), nil
	}

	host := firstNonEmpty(os.Getenv("GAME_POSTGRES_HOST"), os.Getenv("LEVEL_POSTGRES_HOST"), os.Getenv("POSTGRES_HOST"), os.Getenv("AWS_RDS_POSTGRES_HOST"))
	port := firstNonEmpty(os.Getenv("GAME_POSTGRES_PORT"), os.Getenv("LEVEL_POSTGRES_PORT"), os.Getenv("POSTGRES_PORT"), "5432")
	dbName := firstNonEmpty(os.Getenv("GAME_POSTGRES_DB"), os.Getenv("LEVEL_POSTGRES_DB"), os.Getenv("POSTGRES_DB"))
	user := firstNonEmpty(os.Getenv("GAME_POSTGRES_USER"), os.Getenv("LEVEL_POSTGRES_USER"), os.Getenv("POSTGRES_USER"))
	password := firstNonEmpty(os.Getenv("GAME_POSTGRES_PASSWORD"), os.Getenv("LEVEL_POSTGRES_PASSWORD"), os.Getenv("POSTGRES_PASSWORD"))
	sslMode := firstNonEmpty(os.Getenv("GAME_POSTGRES_SSLMODE"), os.Getenv("LEVEL_POSTGRES_SSLMODE"), os.Getenv("POSTGRES_SSLMODE"), "require")
	if host == "" || dbName == "" || user == "" || password == "" {
		return "", errors.New("set GAME_DATABASE_URL, LEVEL_DATABASE_URL, DATABASE_URL, or Postgres connection parts")
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
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func envCSV(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
