package main

import (
	"context"
	"cortex-task/internal/article"
	"cortex-task/internal/config"
	"cortex-task/internal/db"
	"cortex-task/internal/ingest"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := log.New(os.Stdout, "[news-sync] ", log.LstdFlags|log.Lshortfile)

	cfg, err := config.FromEnv()
	if err != nil {
		logger.Fatalf("failed to load config: %v", err)
	}

	mongoClient, err := db.ConnectMongo(ctx, cfg.MongoURI)
	if err != nil {
		logger.Fatalf("failed to connect to db: %v", err)
	}
	defer mongoClient.Disconnect(context.Background())

	dbInstance := mongoClient.Database(cfg.MongoDBName)

	articleRepo, err := article.NewMongoArticleRepository(dbInstance, logger)
	if err != nil {
		logger.Fatalf("failed to init repository: %v", err)
	}

	logger.Printf("article repository initialised: %+v\n", articleRepo)

	httpClient := &http.Client{Timeout: cfg.Timeout}
	feedClient := ingest.NewECBClient(cfg.FeedURL, httpClient)

	// Create ingest service
	ingestService := ingest.NewService(
		articleRepo,
		feedClient,
		cfg.PageSize,
		cfg.MaxPages,
		cfg.MaxPolls,
		logger,
	)

	// Start polling loop in goroutine
	go ingestService.StartPolling(ctx, cfg.PollInterval)

	// Block until shutdown signal
	<-ctx.Done()
	logger.Println("service shutting down...")
	logger.Println("service started (no ingest wired yet)")
}
