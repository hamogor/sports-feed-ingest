package event

import (
	"context"
	"cortex-task/internal/article"
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type Publisher interface {
	PublishArticleUpdated(ctx context.Context, a *article.Article) error
}

type Service struct {
	col       *mongo.Collection
	publisher Publisher
	logger    *log.Logger
}

func NewService(col *mongo.Collection, publisher Publisher, logger *log.Logger) *Service {
	if logger == nil {
		logger = log.Default()
	}

	return &Service{
		col:       col,
		publisher: publisher,
		logger:    logger,
	}
}

func (s *Service) Run(ctx context.Context) {
	stream, err := s.col.Watch(ctx, mongo.Pipeline{})
	if err != nil {
		s.logger.Printf("events: failed to open change stream: %v", err)
		return
	}
	defer stream.Close(ctx)

	s.logger.Println("events: watching MongoDB change stream...")

	for stream.Next(ctx) {
		var event bson.M
		if err := stream.Decode(&event); err != nil {
			s.logger.Printf("events: failed decoding change event: %v", err)
			continue
		}

		externalID := extractExternalID(event)
		if externalID == 0 {
			s.logger.Printf("events: skip event missing externalId: %v", event)
			continue
		}

		var a article.Article
		if err := s.col.FindOne(ctx, bson.M{"externalId": externalID}).Decode(&a); err != nil {
			s.logger.Printf("events: failed fetching updated article %d: %v", externalID, err)
			continue
		}

		if err := s.publisher.PublishArticleUpdated(ctx, &a); err != nil {
			s.logger.Printf("events: failed publishing article %d: %v", externalID, err)
			continue
		}

		s.logger.Printf("events: published article %d to message bus", externalID)
	}

	if err := stream.Err(); err != nil {
		s.logger.Printf("events: change stream closed with error: %v", err)
	} else {
		s.logger.Println("events: change stream stopped")
	}
}

func extractExternalID(event bson.M) int64 {
	if docKey, ok := event["documentKey"].(bson.M); ok {
		if id, ok := docKey["externalId"].(int64); ok {
			return id
		}
	}
	return 0
}
