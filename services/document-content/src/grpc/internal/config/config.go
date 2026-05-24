package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL string
	Host        string
	Port        string
}

func Load() (Config, error) {
	databaseURL, err := databaseURLFromEnv()
	if err != nil {
		return Config{}, err
	}
	return Config{
		DatabaseURL: databaseURL,
		Host:        envOrDefault("CONTENT_GRPC_HOST", "0.0.0.0"),
		Port:        envOrDefault("CONTENT_GRPC_PORT", "50051"),
	}, nil
}

func databaseURLFromEnv() (string, error) {
	if raw := strings.TrimSpace(os.Getenv("DATABASE_URL")); raw != "" {
		return normalizePostgresURL(raw), nil
	}

	host := firstNonEmpty(os.Getenv("POSTGRES_HOST"), os.Getenv("AWS_RDS_POSTGRES_HOST"))
	port := firstNonEmpty(os.Getenv("POSTGRES_PORT"), "5432")
	dbName := os.Getenv("POSTGRES_DB")
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	sslMode := firstNonEmpty(os.Getenv("POSTGRES_SSLMODE"), "require")
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
