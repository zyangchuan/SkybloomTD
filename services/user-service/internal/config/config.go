package config

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Port                string
	DatabaseURL         string
	SupabaseURL         string
	SupabaseJWTSecret   string
	SupabaseJWTAudience string
	SupabaseJWKSURL     string
	AuthCookieName      string
	AllowUnverifiedJWT  bool
	CORSAllowedOrigins  []string
}

func Load() (Config, error) {
	databaseURL, err := databaseURLFromEnv()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Port:                envOrDefault("USER_SERVICE_PORT", envOrDefault("PORT", "8080")),
		DatabaseURL:         databaseURL,
		SupabaseURL:         strings.TrimRight(firstNonEmpty(os.Getenv("SUPABASE_URL"), os.Getenv("NEXT_PUBLIC_SUPABASE_URL")), "/"),
		SupabaseJWTSecret:   strings.TrimSpace(os.Getenv("SUPABASE_JWT_SECRET")),
		SupabaseJWTAudience: envOrDefault("SUPABASE_JWT_AUDIENCE", "authenticated"),
		SupabaseJWKSURL:     supabaseJWKSURL(),
		AuthCookieName:      envOrDefault("AUTH_ACCESS_TOKEN_COOKIE", "skybloom_access_token"),
		AllowUnverifiedJWT:  envBool("ALLOW_UNVERIFIED_JWT", false),
		CORSAllowedOrigins:  csvEnv("CORS_ALLOWED_ORIGINS", "*"),
	}, nil
}

func supabaseJWKSURL() string {
	if value := strings.TrimSpace(os.Getenv("SUPABASE_JWKS_URL")); value != "" {
		return value
	}
	supabaseURL := strings.TrimRight(firstNonEmpty(
		os.Getenv("SUPABASE_URL"),
		os.Getenv("NEXT_PUBLIC_SUPABASE_URL"),
	), "/")
	if supabaseURL == "" {
		return ""
	}
	return supabaseURL + "/auth/v1/.well-known/jwks.json"
}

func databaseURLFromEnv() (string, error) {
	if raw := strings.TrimSpace(os.Getenv("DATABASE_URL")); raw != "" {
		return normalizePostgresURL(raw), nil
	}

	host := strings.TrimSpace(firstNonEmpty(
		os.Getenv("USER_POSTGRES_HOST"),
		os.Getenv("POSTGRES_HOST"),
		os.Getenv("AWS_RDS_POSTGRES_HOST"),
	))
	port := strings.TrimSpace(firstNonEmpty(
		os.Getenv("USER_POSTGRES_PORT"),
		os.Getenv("POSTGRES_PORT"),
		"5432",
	))
	dbName := strings.TrimSpace(firstNonEmpty(
		os.Getenv("USER_POSTGRES_DB"),
		os.Getenv("POSTGRES_DB"),
	))
	user := strings.TrimSpace(firstNonEmpty(
		os.Getenv("USER_POSTGRES_USER"),
		os.Getenv("POSTGRES_USER"),
	))
	password := firstNonEmpty(
		os.Getenv("USER_POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_PASSWORD"),
	)
	sslMode := strings.TrimSpace(firstNonEmpty(
		os.Getenv("USER_POSTGRES_SSLMODE"),
		os.Getenv("POSTGRES_SSLMODE"),
		"require",
	))

	if host == "" || dbName == "" || user == "" || password == "" {
		return "", errors.New("set DATABASE_URL or POSTGRES_HOST, POSTGRES_DB, POSTGRES_USER, and POSTGRES_PASSWORD")
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   host + ":" + port,
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

func envBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func csvEnv(key string, fallback string) []string {
	raw := envOrDefault(key, fallback)
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return []string{fallback}
	}
	return values
}
