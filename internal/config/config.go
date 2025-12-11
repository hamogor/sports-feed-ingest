package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	MongoURI         string
	MongoDBName      string
	FeedURL          string
	PageSize         int
	MaxPages         int // -1 to ingest all pages
	MaxPolls         int // -1 is unlimited
	Timeout          time.Duration
	PollInterval     time.Duration
	RabbitURI        string
	RabbitExchange   string
	RabbitRoutingKey string
}

const (
	MongoURI            = "MONGO_URI"
	MongoDBName         = "MONGO_DB_NAME"
	FeedURL             = "FEED_URL"
	PageSize            = "PAGE_SIZE"
	MaxPolls            = "MAX_POLLS"
	MaxPages            = "MAX_PAGES"
	Timeout             = "TIMEOUT"
	PollInterval        = "POLL_INTERVAL"
	RabbitURIEnv        = "RABBIT_URI"
	RabbitExchangeEnv   = "RABBIT_EXCHANGE"
	RabbitRoutingKeyEnv = "RABBIT_ROUTING_KEY"
)

func FromEnv() (Config, error) {
	var cfg Config

	cfg.MongoURI = getEnv(MongoURI, "mongodb://localhost:27017")
	cfg.MongoDBName = getEnv(MongoDBName, "newsdb")
	cfg.FeedURL = getEnv(FeedURL, "https://content-ecb.pulselive.com/content/ecb/text/EN/")
	cfg.RabbitURI = getEnv(RabbitURIEnv, "amqp://guest:guest@localhost:5672/")
	cfg.RabbitExchange = getEnv(RabbitExchangeEnv, "cms.sync")
	cfg.RabbitRoutingKey = getEnv(RabbitRoutingKeyEnv, "article.updated")

	var err error
	if cfg.PageSize, err = getEnvInt(PageSize, 20); err != nil {
		return cfg, fmt.Errorf("invalid %v: %w", PageSize, err)
	}
	if cfg.MaxPolls, err = getEnvInt(MaxPolls, -1); err != nil {
		return cfg, fmt.Errorf("invalid %v: %w", MaxPolls, err)
	}
	if cfg.MaxPages, err = getEnvInt(MaxPages, -1); err != nil {
		return cfg, fmt.Errorf("invalid %v: %w", MaxPages, err)
	}
	timeoutStr := getEnv(Timeout, "10s")
	if cfg.Timeout, err = time.ParseDuration(timeoutStr); err != nil {
		return cfg, fmt.Errorf("invalid %v: %w", Timeout, err)
	}
	pollIntervalStr := getEnv(PollInterval, "1s")
	if cfg.PollInterval, err = time.ParseDuration(pollIntervalStr); err != nil {
		return cfg, fmt.Errorf("invalid %v: %w", PollInterval, err)
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
