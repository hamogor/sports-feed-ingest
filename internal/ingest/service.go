package ingest

import (
	"context"
	"cortex-task/internal/article"
	"log"
	"time"
)

type ArticleRepo interface {
	UpsertByExternalID(ctx context.Context, a *article.Article) (bool, error)
}

type FeedClient interface {
	FetchPage(ctx context.Context, page, pageSize int) (ECBResponse, error)
}

type Service struct {
	repo     ArticleRepo
	client   FeedClient
	pageSize int
	maxPages int
	maxPolls int
	logger   *log.Logger
}

func NewService(repo ArticleRepo, client FeedClient, pageSize, maxPages, maxPolls int, logger *log.Logger) *Service {
	return &Service{
		repo:     repo,
		client:   client,
		pageSize: pageSize,
		maxPages: maxPages,
		maxPolls: maxPolls,
		logger:   logger,
	}
}

func (s *Service) RunOnce(ctx context.Context) error {
	page := 0                     // current page
	emptyCount := 0               // how many times we've seen an empty page (in a row)
	const AbsoluteMaxPages = 5000 // absolute max amount of pages we can ingest.

	for {
		resp, err := s.client.FetchPage(ctx, page, s.pageSize)
		if err != nil {
			return err
		}

		// Found an empty page, increment `emptyPage`
		if len(resp.Content) == 0 {
			emptyCount++
			if emptyCount >= 3 {
				s.logger.Println("no content for 3 pages — stopping")
				return nil
			}
		} else {
			// We found a page with data so reset
			emptyCount = 0
		}

		for _, ecbArt := range resp.Content {
			art, err := MapECBToArticle(ecbArt)
			if err != nil {
				s.logger.Printf("mapping failed for %d: %v", ecbArt.ID, err)
				continue
			}
			updated, err := s.repo.UpsertByExternalID(ctx, &art)
			if err != nil {
				log.Printf("failed to upsert for %d: %v", ecbArt.ID, err)
			}
			if updated {
				log.Printf("updated article: %v", art.ExternalID)
			}
		}

		page++

		// Not a huge fan of this...
		if page >= AbsoluteMaxPages {
			s.logger.Printf("safety stop: %d pages scanned", AbsoluteMaxPages)
			return nil
		}

		if s.maxPages >= 0 && page >= s.maxPages {
			s.logger.Printf("reached configured page limit %d", s.maxPages)
			return nil
		}

		if page >= resp.PageInfo.NumPages {
			s.logger.Printf("reached reported last page %d", resp.PageInfo.NumPages)
			return nil
		}
	}
}

func (s *Service) StartPolling(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	pollCount := 0

	s.logger.Printf("polling every %v...", interval)

	for {
		select {
		case <-ctx.Done():
			s.logger.Println("poller stopping — context cancelled")
			return

		case <-ticker.C:
			pollCount++

			// EXTRA GUARDBLOCK: stop after max polls
			if s.maxPolls > 0 && pollCount >= s.maxPolls {
				s.logger.Printf("poller stopping after %d polls (max reached)", pollCount)
				return
			}

			s.logger.Printf("poll #%d starting ingestion...", pollCount)

			// TIME-LIMIT THE INVOCATION
			pollCtx, cancel := context.WithTimeout(ctx, 25*time.Minute)

			if err := s.RunOnce(pollCtx); err != nil {
				s.logger.Printf("poll error: %v", err)
			}

			cancel()
		}
	}
}
