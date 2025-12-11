package event

import (
	"context"
	"cortex-task/internal/article"
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

		docID, ok := extractDocumentID(event)
		if !ok || docID == primitive.NilObjectID {
			s.logger.Printf("events: skip event missing _id in documentKey")
			continue
		}

		var a article.Article
		if err := s.col.FindOne(ctx, bson.M{"_id": docID}).Decode(&a); err != nil {
			s.logger.Printf("events: failed fetching updated article %s: %v", docID.Hex(), err)
			continue
		}

		if err := s.publisher.PublishArticleUpdated(ctx, &a); err != nil {
			s.logger.Printf("events: failed publishing article %d: %v", a.ExternalID, err)
			continue
		}

		s.logger.Printf("events: published article %d to message bus", a.ExternalID)
	}

	if err := stream.Err(); err != nil {
		s.logger.Printf("events: change stream closed with error: %v", err)
	} else {
		s.logger.Println("events: change stream stopped")
	}
}

// extractDocumentID extracts the MongoDB _id from the change stream event's documentKey.
// Change streams always include _id in documentKey, not arbitrary fields like externalId.
func extractDocumentID(event bson.M) (primitive.ObjectID, bool) {
	docKey, ok := event["documentKey"].(bson.M)
	if !ok {
		return primitive.NilObjectID, false
	}

	id, ok := docKey["_id"].(primitive.ObjectID)
	if !ok {
		return primitive.NilObjectID, false
	}

	return id, true
}
