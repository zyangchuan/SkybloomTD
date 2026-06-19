package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port string
	DatabaseURL string
	RabbitMQURL string
	DocumentContentQueue string
	RedisURL string
	TaskStatusTTL time.Duration
	S3Bucket string
	S3Endpoint string
	S3AccessKey string
	S3SecretKey string
	S3Region string
}

func Load() (Config, error) {

	databaseURL, err := databaseURLFromEnv()
	if err != nil {
		return Config{}, err
	}

	mq := os.Getenv("RABBITMQ_URL")
	redis := os.Getenv("REDIS_URL")
	taskStatusTTLSeconds := os.Getenv("TASK_STATUS_TTL_SECONDS")
	bucket := os.Getenv("AWS_S3_BUCKET")
	endpoint := os.Getenv("AWS_S3_ENDPOINT_URL")
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	region := os.Getenv("AWS_REGION")

	if mq == "" || redis == "" || taskStatusTTLSeconds == "" ||
		bucket == "" || endpoint == "" ||
		accessKey == "" || secretKey == "" || region == "" {
		return Config{}, errors.New("missing environment variables")
	}

	taskStatusTTL, err := strconv.Atoi(taskStatusTTLSeconds)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Port:                 "8000",
		DatabaseURL:          databaseURL,
		RabbitMQURL:          mq,
		DocumentContentQueue: "document-content-queue",
		RedisURL:             redis,
		TaskStatusTTL:        time.Duration(taskStatusTTL) * time.Second,
		S3Bucket:             bucket,
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
