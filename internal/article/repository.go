package article

import (
	"context"
	"errors"
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

	if err != nil && r.logger != nil {
		r.logger.Printf("failed to create indexes: %v", err)
	}
	return err
}

// UpsertByExternalID uses the ID from content-ecb feed to upsert data within db
// returns true if an article was created / updated.
func (r *mongoRepository) UpsertByExternalID(ctx context.Context, a *Article) (bool, error) {
	now := time.Now()

	res := r.col.FindOne(ctx, bson.M{"externalId": a.ExternalID})
	if errors.Is(res.Err(), mongo.ErrNoDocuments) {
		if r.logger != nil {
			r.logger.Printf("inserting new article: %d", a.ExternalID)
		}

		a.CreatedAt = now
		a.ModifiedAt = now

		_, err := r.col.InsertOne(ctx, a)
		if err != nil {
			return false, err
		}

		return true, nil
	}

	if res.Err() != nil {
		return false, res.Err()
	}

	existing := Article{}
	err := res.Decode(&existing)
	if err != nil {
		return false, err
	}

	shouldUpdateArticle := !a.LastModified.IsZero() && a.LastModified.After(existing.LastModified)
	shouldUpdateMedia := !a.LeadMedia.LastModified.IsZero() && a.LeadMedia.LastModified.After(existing.LeadMedia.LastModified)

	if !shouldUpdateArticle && !shouldUpdateMedia {
		return false, nil
	}

	set := bson.M{}
	if shouldUpdateArticle {
		if r.logger != nil {
			r.logger.Printf("updating article with newer lastModified: %v", a.ExternalID)
		}

		set["type"] = a.Type
		set["title"] = a.Title
		set["description"] = a.Description
		set["date"] = a.Date
		set["location"] = a.Location
		set["language"] = a.Language
		set["canonicalUrl"] = a.CanonicalURL
		set["lastModified"] = a.LastModified
		set["body"] = a.Body
		set["summary"] = a.Summary
	}

	if shouldUpdateMedia {
		if r.logger != nil {
			r.logger.Printf("updating lead media with newer lastModified: %v", a.ExternalID)
		}

		set["leadMedia"] = bson.M{
			"id":           a.LeadMedia.ID,
			"type":         a.LeadMedia.Type,
			"title":        a.LeadMedia.Title,
			"date":         a.LeadMedia.Date,
			"language":     a.LeadMedia.Language,
			"imageUrl":     a.LeadMedia.ImageURL,
			"lastModified": a.LeadMedia.LastModified,
		}
	}

	set["modifiedAt"] = now

	_, err = r.col.UpdateOne(
		ctx,
		bson.M{"externalId": a.ExternalID},
		bson.M{"$set": set},
	)
	if err != nil {
		return false, err
	}

	return true, nil
}
