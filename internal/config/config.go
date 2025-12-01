package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	MongoURI     string
	MongoDBName  string
	FeedURL      string
	PageSize     int
	MaxPages     int // -1 to ingest all pages
	MaxPolls     int // -1 is unlimited
	Timeout      time.Duration
	PollInterval time.Duration
}

const (
	MongoURI    = "MONGO_URI"
	MongoDBName = "MONGO_DB_NAME"
	FeedURL     = "FEED_URL"
	PageSize    = "PAGE_SIZE"
)

func FromEnv() (Config, error) {
	var cfg Config

	cfg.MongoURI = getEnv(MongoURI, "mongodb://localhost:27017")
	cfg.MongoDBName = getEnv(MongoDBName, "newsdb")
	cfg.FeedURL = getEnv(FeedURL, "https://content-ecb.pulselive.com/content/ecb/text/EN/")

	cfg.MaxPolls = -1
	cfg.PageSize = 20
	cfg.MaxPages = 100
	cfg.Timeout = time.Second * 10
	cfg.PollInterval = time.Second * 5

	var err error
	if cfg.PageSize, err = getEnvInt("PAGE_SIZE", 20); err != nil {
		return cfg, fmt.Errorf("invalid PAGE_SIZE: %w", err)
	}
	if cfg.MaxPages, err = getEnvInt("MAX_PAGES", 5); err != nil {
		return cfg, fmt.Errorf("invalid MAX_PAGES: %w", err)
	}
	timeoutStr := getEnv("HTTP_TIMEOUT", "10s")
	if cfg.Timeout, err = time.ParseDuration(timeoutStr); err != nil {
		return cfg, fmt.Errorf("invalid HTTP_TIMEOUT: %w", err)
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0, err
	}
	return i, nil
}
