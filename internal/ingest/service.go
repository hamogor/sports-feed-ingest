package ingest

import (
	"context"
	"cortex-task/internal/article"
	"log"
	"time"
)

const AbsoluteMaxPages = 5000 // absolute max amount of pages we can ingest

type FeedClient interface {
	FetchPage(ctx context.Context, page, pageSize int) (ECBResponse, error)
}

// ticker is an interface so we can swap out time.Ticker in tests.
type ticker interface {
	C() <-chan time.Time
	Stop()
}

type tickerFactory func(d time.Duration) ticker

// timeTicker is the real implementation backed by time.Ticker.
type timeTicker struct {
	*time.Ticker
}

func (t *timeTicker) C() <-chan time.Time {
	return t.Ticker.C
}

func (t *timeTicker) Stop() {
	t.Ticker.Stop()
}

type Service struct {
	repo      article.Repository
	client    FeedClient
	pageSize  int
	maxPages  int
	maxPolls  int
	logger    *log.Logger
	newTicker tickerFactory
}

func NewService(repo article.Repository, client FeedClient, pageSize, maxPages, maxPolls int, logger *log.Logger) *Service {
	if logger == nil {
		logger = log.Default()
	}

	return &Service{
		repo:     repo,
		client:   client,
		pageSize: pageSize,
		maxPages: maxPages,
		maxPolls: maxPolls,
		logger:   logger,
		newTicker: func(d time.Duration) ticker {
			return &timeTicker{time.NewTicker(d)}
		},
	}
}

func (s *Service) RunOnce(ctx context.Context) error {
	page := 0                        // current page
	emptyCount := 0                  // how many times we've seen an empty page (in a row)
	seen := make(map[int64]struct{}) // prevent writing articles twice in event of data overlap

	for {
		resp, err := s.client.FetchPage(ctx, page, s.pageSize)
		if err != nil {
			return err
		}

		// Found an empty page, increment `emptyCount`
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

		batch := make([]*article.Article, 0, len(resp.Content))

		for _, ecbArt := range resp.Content {
			if _, ok := seen[ecbArt.ID]; ok {
				continue
			}
			seen[ecbArt.ID] = struct{}{}

			art, err := MapECBToArticle(ecbArt)
			if err != nil {
				s.logger.Printf("mapping failed for %d: %v", ecbArt.ID, err)
				continue
			}
			batch = append(batch, &art)
		}

		if len(batch) > 0 {
			changed, err := s.repo.BulkUpsert(ctx, batch)
			if err != nil {
				s.logger.Printf("bulk upsert failed on page %d: %v", page, err)
			} else {
				s.logger.Printf("bulk upsert: %d documents changed on page %d", changed, page)
			}
		}

		page++

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
	t := s.newTicker(interval)
	defer t.Stop()

	pollCount := 0

	s.logger.Printf("polling every %v...", interval)

	for {
		select {
		case <-ctx.Done():
			s.logger.Println("poller stopping — context cancelled")
			return

		case <-t.C():
			// stop after maxPolls
			if s.maxPolls > 0 && pollCount >= s.maxPolls {
				s.logger.Printf("poller stopping after %d polls (max reached)", pollCount)
				return
			}

			pollCount++
			s.logger.Printf("poll #%d starting ingestion...", pollCount)

			// hard limit
			pollCtx, cancel := context.WithTimeout(ctx, 25*time.Minute)

			if err := s.RunOnce(pollCtx); err != nil {
				s.logger.Printf("poll error: %v", err)
			}

			cancel()
		}
	}
}
