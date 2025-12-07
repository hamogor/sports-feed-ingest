package main

import (
	"context"
	"cortex-task/internal/article"
	"cortex-task/internal/config"
	"cortex-task/internal/db"
	"cortex-task/internal/event"
	"cortex-task/internal/ingest"
	"errors"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Root context cancelled on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := log.New(os.Stdout, "[news-sync] ", log.LstdFlags|log.Lshortfile)

	cfg, err := config.FromEnv()
	if err != nil {
		logger.Fatalf("failed to load config: %v", err)
	}

	// Mongo
	mongoClient, err := db.ConnectMongo(ctx, cfg.MongoURI)
	if err != nil {
		logger.Fatalf("failed to connect to db: %v", err)
	}
	dbInstance := mongoClient.Database(cfg.MongoDBName)

	// Article repository
	articleRepo, err := article.NewMongoArticleRepository(dbInstance, logger)
	if err != nil {
		logger.Fatalf("failed to init repository: %v", err)
	}
	logger.Println("article repository initialised")

	// Feed client
	httpClient := &http.Client{Timeout: cfg.Timeout}
	feedClient := ingest.NewECBClient(cfg.FeedURL, httpClient)

	// Ingest service (poller)
	ingestService := ingest.NewService(
		articleRepo,
		feedClient,
		cfg.PageSize,
		cfg.MaxPages,
		cfg.MaxPolls,
		logger,
	)

	// Event publisher (RabbitMQ)
	publisher, err := event.NewRabbitPublisher(
		cfg.RabbitURI,
		cfg.RabbitExchange,
		cfg.RabbitRoutingKey,
		logger,
	)
	if err != nil {
		logger.Fatalf("failed to init rabbit publisher: %v", err)
	}
	defer publisher.Close()

	eventsService := event.NewService(
		dbInstance.Collection("articles"),
		publisher,
		logger,
	)

	// HTTP health server
	srv := healthz(logger)

	// Start background workers
	go ingestService.StartPolling(ctx, cfg.PollInterval)
	go eventsService.Run(ctx)

	logger.Println("service started")

	// Block until we receive a signal / ctx cancelled
	<-ctx.Done()
	logger.Println("shutdown signal received, shutting down...")

	// Unified shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Graceful HTTP shutdown
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Printf("HTTP server shutdown error: %v", err)
	}

	// Graceful Mongo shutdown
	if err := mongoClient.Disconnect(shutdownCtx); err != nil {
		logger.Printf("mongo disconnect error: %v", err)
	}

	logger.Println("shutdown complete")
}

func healthz(logger *log.Logger) *http.Server {
	r := mux.NewRouter()

	r.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}).Methods(http.MethodGet)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		logger.Printf("HTTP server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Printf("HTTP server error: %v", err)
		}
	}()

	return srv
}
