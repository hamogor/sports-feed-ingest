package main

import (
	"context"
	"cortex-task/internal/article"
	"cortex-task/internal/config"
	"cortex-task/internal/db"
	"cortex-task/internal/event"
	"cortex-task/internal/ingest"
	"github.com/gorilla/mux"
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

	healthz(ctx, logger)

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

	// Create rabbitmq publisher
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

	// Start polling & publishing
	go ingestService.StartPolling(ctx, cfg.PollInterval)
	go eventsService.Run(ctx)

	// Block until shutdown signal
	<-ctx.Done()
	logger.Println("service shutting down...")
	logger.Println("service started (no ingest wired yet)")
}

func healthz(ctx context.Context, logger *log.Logger) *http.Server {
	r := mux.NewRouter()

	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}).Methods("GET")

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		logger.Printf("HTTP server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Printf("HTTP server error: %v", err)
		}
	}()

	// Stop server when context is cancelled
	go func() {
		<-ctx.Done()
		logger.Println("HTTP server shutting down...")
		_ = srv.Shutdown(context.Background())
	}()

	return srv
}
