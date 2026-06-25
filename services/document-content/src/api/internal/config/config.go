package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"time"
)

type Config struct {
	Port                 string
	DatabaseURL          string
	RabbitMQURL          string
	DocumentContentQueue string
	RedisURL             string
	TaskStatusTTL        time.Duration
	S3Endpoint           string
	S3AccessKey          string
	S3SecretKey          string
	S3Region             string
}

func Load() (Config, error) {

	databaseURL, err := databaseURLFromEnv()
	if err != nil {
		return Config{}, err
	}

	endpoint := os.Getenv("AWS_S3_ENDPOINT_URL")
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	region := os.Getenv("AWS_REGION")

	if endpoint == "" || accessKey == "" || secretKey == "" || region == "" {
		return Config{}, errors.New("missing environment variables")
	}

	return Config{
		Port:                 "8000",
		DatabaseURL:          databaseURL,
		RabbitMQURL:          "amqp://guest:guest@rabbitmq:5672/",
		DocumentContentQueue: "document-content-ocr-queue",
		RedisURL:             "redis://redis:6379/0",
		TaskStatusTTL:        7 * 24 * time.Hour,
		S3Endpoint:           endpoint,
		S3AccessKey:          accessKey,
		S3SecretKey:          secretKey,
		S3Region:             region,
	}, nil
}

func databaseURLFromEnv() (string, error) {
	host := os.Getenv("POSTGRES_HOST")
	port := os.Getenv("POSTGRES_PORT")
	dbName := os.Getenv("POSTGRES_DB")
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	sslMode := os.Getenv("POSTGRES_SSLMODE")

	if host == "" || port == "" || dbName == "" ||
		user == "" || password == "" || sslMode == "" {
		return "", errors.New("missing PostgreSQL database env variables.")
	}

	// Constructing postgres database URL
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + dbName,
	}

	query := u.Query()
	query.Set("sslmode", sslMode)
	u.RawQuery = query.Encode()

	return u.String(), nil
}
