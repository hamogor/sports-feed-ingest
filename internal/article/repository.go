package article

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Repository interface {
	UpsertByExternalID(ctx context.Context, a *Article) (bool, error)
}

type mongoRepository struct {
	col    *mongo.Collection
	logger *log.Logger
}

func NewMongoArticleRepository(db *mongo.Database, logger *log.Logger) (Repository, error) {
	col := db.Collection("articles")

	repo := &mongoRepository{
		col:    col,
		logger: logger,
	}
	if err := repo.ensureIndexes(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

// ensureIndexes ensures that no article that shares a unique id from content-ecb (externalID)
// can be inserted into db twice. it also ensures that data is ordered by `lastModified`
func (r *mongoRepository) ensureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "externalId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "lastModified", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "leadMedia.lastModified", Value: 1}},
		},
	}
	_, err := r.col.Indexes().CreateMany(ctx, indexes)
	return err
}

// UpsertByExternalID uses the ID from content-ecb feed to upsert data within db
// returns true if an article was created / updated.
func (r *mongoRepository) UpsertByExternalID(ctx context.Context, a *Article) (bool, error) {
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.ModifiedAt = now

	filter := bson.M{"externalId": a.ExternalID}

	update := bson.M{
		"$set": bson.M{
			"type":         a.Type,
			"title":        a.Title,
			"description":  a.Description,
			"date":         a.Date,
			"location":     a.Location,
			"language":     a.Language,
			"canonicalUrl": a.CanonicalURL,
			"lastModified": a.LastModified,

			"body":    a.Body,
			"summary": a.Summary,

			"leadMedia": bson.M{
				"id":           a.LeadMedia.ID,
				"type":         a.LeadMedia.Type,
				"title":        a.LeadMedia.Title,
				"date":         a.LeadMedia.Date,
				"language":     a.LeadMedia.Language,
				"imageUrl":     a.LeadMedia.ImageURL,
				"lastModified": a.LeadMedia.LastModified,
			},

			"modifiedAt": a.ModifiedAt,
		},
		"$setOnInsert": bson.M{
			"createdAt": a.CreatedAt,
		},
	}

	res, err := r.col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		return false, err
	}

	// new insert or actual update
	changed := res.UpsertedCount == 1 || res.ModifiedCount == 1
	return changed, nil
}
