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
	BulkUpsert(ctx context.Context, articles []*Article) (int, error)
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

	if err != nil {
		r.logger.Printf("failed to create indexes: %v", err)
	}
	return err
}

func (r *mongoRepository) BulkUpsert(ctx context.Context, articles []*Article) (int, error) {
	if len(articles) == 0 {
		return 0, nil
	}

	// collect ids
	ids := make([]int64, 0, len(articles))

	for _, a := range articles {
		ids = append(ids, a.ExternalID)
	}

	// load the existing docs
	cur, err := r.col.Find(ctx, bson.M{"externalId": bson.M{"$in": ids}})
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)

	existingID := make(map[int64]Article, len(articles))
	for cur.Next(ctx) {
		var ex Article
		if err := cur.Decode(&ex); err != nil {
			return 0, err
		}
		existingID[ex.ExternalID] = ex
	}
	if err := cur.Err(); err != nil {
		return 0, err
	}

	// build the bulk write based on lastModified rules
	now := time.Now()
	models := make([]mongo.WriteModel, 0, len(articles))

	for _, a := range articles {
		ex, found := existingID[a.ExternalID]
		if !found {
			// new document insert
			a.CreatedAt = now
			a.ModifiedAt = now
			models = append(models, mongo.NewInsertOneModel().SetDocument(a))
			continue
		}

		// existing doc, decide if we need to update the whole article or the lead media, or both
		shouldUpdateArticle := !a.LastModified.IsZero() && a.LastModified.After(ex.LastModified)
		shouldUpdateMedia := !a.LeadMedia.LastModified.IsZero() && a.LeadMedia.LastModified.After(ex.LeadMedia.LastModified)

		if !shouldUpdateMedia && !shouldUpdateArticle {
			continue // article hasn't changed
		}

		set := bson.M{}
		if shouldUpdateArticle {
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

		update := bson.M{"$set": set}
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"externalId": a.ExternalID}).
			SetUpdate(update),
		)
	}

	if len(models) == 0 {
		return 0, nil
	}

	// do bulk write (unordered for concurrency and to prevent stopping on a single failure)
	res, err := r.col.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
	if err != nil {
		return 0, err
	}

	return int(res.InsertedCount + res.ModifiedCount + res.UpsertedCount), nil
}
