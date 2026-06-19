package config

import (
	"errors"
	"net"
	"net/url"
	"os"
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
		Host:        "0.0.0.0",
		Port:        "50051",
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
		return "", errors.New("missing PostgreSQL database env variables")
	}

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
