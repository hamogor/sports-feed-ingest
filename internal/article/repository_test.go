package article_test

import (
	"context"
	"testing"
	"time"

	"cortex-task/internal/article"
	"cortex-task/internal/db"

	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/mongo"
)

type ArticleIngestionSuite struct {
	suite.Suite

	ctx    context.Context
	client *mongo.Client
	db     *mongo.Database
	col    *mongo.Collection

	repo article.Repository
}

func TestArticleIngestionSuite(t *testing.T) {
	suite.Run(t, new(ArticleIngestionSuite))
}

func (s *ArticleIngestionSuite) SetupSuite() {
	s.ctx = context.Background()

	mongoURI := "mongodb://localhost:27017"
	mongoDBName := "test_newsdb"

	client, err := db.ConnectMongo(s.ctx, mongoURI)
	s.Require().NoError(err, "failed to connect to mongo")
	s.client = client

	database := client.Database(mongoDBName)
	s.db = database
	s.col = database.Collection("articles")

	repo, err := article.NewMongoArticleRepository(database, nil)
	s.Require().NoError(err, "failed to create article repository")
	s.repo = repo
}

func (s *ArticleIngestionSuite) TearDownSuite() {
	if s.client != nil {
		_ = s.client.Disconnect(s.ctx)
	}
}

func (s *ArticleIngestionSuite) SetupTest() {
	// ensure a fresh DB before each test
	_ = s.db.Drop(s.ctx)
}

func (s *ArticleIngestionSuite) TestArticleIngestionEndToEnd() {
	// insert one
	A1 := article.Article{
		ExternalID:   1001,
		Title:        "First Title",
		Description:  "First Description",
		LastModified: time.Unix(1700000000, 0),
		LeadMedia: article.LeadMedia{
			ID:           2001,
			Title:        "First Lead",
			LastModified: time.Unix(1700000000, 0),
		},
	}

	changed, err := s.repo.BulkUpsert(s.ctx, []*article.Article{&A1})
	s.Require().NoError(err)
	s.Require().Equal(1, changed, "first insert, change 1 document")

	var gotA1 article.Article
	err = s.col.FindOne(s.ctx, map[string]any{"externalId": 1001}).Decode(&gotA1)
	s.Require().NoError(err)
	s.Equal("Title", gotA1.Title)
	s.Equal(time.Unix(1700000000, 0), gotA1.LastModified)

	// insert new article
	A2 := article.Article{
		ExternalID:   1002,
		Title:        "Second Title",
		LastModified: time.Unix(1700000100, 0),
		LeadMedia: article.LeadMedia{
			ID:           2002,
			LastModified: time.Unix(1700000100, 0),
		},
	}

	changed, err = s.repo.BulkUpsert(s.ctx, []*article.Article{&A2})
	s.Require().NoError(err)
	s.Require().Equal(1, changed, "second insert, change 1 document")

	var gotA2 article.Article
	err = s.col.FindOne(s.ctx, map[string]any{"externalId": 1002}).Decode(&gotA2)
	s.Require().NoError(err)
	s.Equal("Second Title", gotA2.Title)

	// update first article with new last modified
	A3 := article.Article{
		ExternalID:   1001,
		Title:        "Updated Title",
		Description:  "Updated Description",
		LastModified: time.Unix(1700000500, 0), // new timestamp
		LeadMedia: article.LeadMedia{
			ID:           2001,
			Title:        "First Lead",
			LastModified: time.Unix(1700000000, 0),
		},
	}

	changed, err = s.repo.BulkUpsert(s.ctx, []*article.Article{&A3})
	s.Require().NoError(err)
	s.Require().Equal(1, changed, "newer article LastModified should trigger 1 update")

	var gotA3 article.Article
	err = s.col.FindOne(s.ctx, map[string]any{"externalId": 1001}).Decode(&gotA3)
	s.Require().NoError(err)
	s.Equal("Updated Title", gotA3.Title)
	s.Equal(time.Unix(1700000500, 0), gotA3.LastModified)
	// lead media should still be the old one at this point
	s.Equal("Lead A1", gotA3.LeadMedia.Title)
	s.Equal(time.Unix(1700000000, 0), gotA3.LeadMedia.LastModified)

	// update first article with new lead
	A4 := article.Article{
		ExternalID:   1001,
		Title:        "Updated Title",          // unchanged
		Description:  "Updated Description",    // unchanged
		LastModified: time.Unix(1700000500, 0), // same as A3
		LeadMedia: article.LeadMedia{
			ID:           2001,
			Title:        "Lead UPDATED",           // changed
			LastModified: time.Unix(1700000900, 0), // NEWER
		},
	}

	changed, err = s.repo.BulkUpsert(s.ctx, []*article.Article{&A4})
	s.Require().NoError(err)
	s.Require().Equal(1, changed, "newer leadMedia LastModified should trigger 1 update")

	var gotA4 article.Article
	err = s.col.FindOne(s.ctx, map[string]any{"externalId": 1001}).Decode(&gotA4)
	s.Require().NoError(err)

	// article fields unchanged from A3
	s.Equal("Updated Title", gotA4.Title)
	s.Equal("Updated Description", gotA4.Description)
	s.Equal(time.Unix(1700000500, 0), gotA4.LastModified)

	// lead media updated from A4
	s.Equal("Lead UPDATED", gotA4.LeadMedia.Title)
	s.Equal(time.Unix(1700000900, 0), gotA4.LeadMedia.LastModified)
}
