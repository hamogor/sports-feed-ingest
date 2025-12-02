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

type articleRepo interface {
	UpsertByExternalID(ctx context.Context, a *article.Article) (bool, error)
}

type ArticleIngestionSuite struct {
	suite.Suite

	ctx    context.Context
	client *mongo.Client
	db     *mongo.Database
	col    *mongo.Collection

	repo articleRepo
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

	// initial insert
	A1 := article.Article{
		ExternalID:   1001,
		Title:        "Original Title",
		Description:  "Original Description",
		LastModified: time.Unix(1700000000, 0),
		LeadMedia: article.LeadMedia{
			ID:           2001,
			Title:        "Lead A1",
			LastModified: time.Unix(1700000000, 0),
		},
	}

	changed, err := s.repo.UpsertByExternalID(s.ctx, &A1)
	s.Require().NoError(err)
	s.Require().True(changed)

	var gotA1 article.Article
	err = s.col.FindOne(s.ctx, map[string]any{"externalId": 1001}).Decode(&gotA1)
	s.Require().NoError(err)
	s.Equal("Original Title", gotA1.Title)
	s.Equal(time.Unix(1700000000, 0), gotA1.LastModified)

	// insert different article
	A2 := article.Article{
		ExternalID:   1002,
		Title:        "Second Title",
		LastModified: time.Unix(1700000100, 0),
		LeadMedia: article.LeadMedia{
			ID:           2002,
			LastModified: time.Unix(1700000100, 0),
		},
	}

	changed, err = s.repo.UpsertByExternalID(s.ctx, &A2)
	s.Require().NoError(err)
	s.Require().True(changed)

	var gotA2 article.Article
	err = s.col.FindOne(s.ctx, map[string]any{"externalId": 1002}).Decode(&gotA2)
	s.Require().NoError(err)
	s.Equal("Second Title", gotA2.Title)

	// update A1
	A3 := article.Article{
		ExternalID:   1001,
		Title:        "Updated Title",
		Description:  "Updated Description",
		LastModified: time.Unix(1700000500, 0),
		LeadMedia: article.LeadMedia{
			ID:           2001,
			Title:        "Lead A1",
			LastModified: time.Unix(1700000000, 0),
		},
	}

	changed, err = s.repo.UpsertByExternalID(s.ctx, &A3)
	s.Require().NoError(err)
	s.Require().True(changed)

	var gotA3 article.Article
	err = s.col.FindOne(s.ctx, map[string]any{"externalId": 1001}).Decode(&gotA3)
	s.Require().NoError(err)
	s.Equal("Updated Title", gotA3.Title)
	s.Equal(time.Unix(1700000500, 0), gotA3.LastModified)

	// update only leadMedia
	A4 := article.Article{
		ExternalID:   1001,
		Title:        "Updated Title",
		Description:  "Updated Description",
		LastModified: time.Unix(1700000500, 0),
		LeadMedia: article.LeadMedia{
			ID:           2001,
			Title:        "Lead UPDATED",
			LastModified: time.Unix(1700000900, 0),
		},
	}

	changed, err = s.repo.UpsertByExternalID(s.ctx, &A4)
	s.Require().NoError(err)
	s.Require().True(changed)

	var gotA4 article.Article
	err = s.col.FindOne(s.ctx, map[string]any{"externalId": 1001}).Decode(&gotA4)
	s.Require().NoError(err)

	s.Equal("Updated Title", gotA4.Title)
	s.Equal(time.Unix(1700000500, 0), gotA4.LastModified)

	s.Equal("Lead UPDATED", gotA4.LeadMedia.Title)
	s.Equal(time.Unix(1700000900, 0), gotA4.LeadMedia.LastModified)
}
