package ingest

import (
	"bytes"
	"context"
	"cortex-task/internal/article"
	"errors"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

// -------------------------
// Mocks
// -------------------------

type mockArticleRepo struct {
	mock.Mock
}

func (m *mockArticleRepo) BulkUpsert(ctx context.Context, articles []*article.Article) (int, error) {
	args := m.Called(ctx, articles)
	return args.Int(0), args.Error(1)
}

type mockFeedClient struct {
	mock.Mock
}

func (m *mockFeedClient) FetchPage(ctx context.Context, page, pageSize int) (ECBResponse, error) {
	args := m.Called(ctx, page, pageSize)
	return args.Get(0).(ECBResponse), args.Error(1)
}

type fakeTicker struct {
	ch chan time.Time
}

func (f *fakeTicker) C() <-chan time.Time { return f.ch }
func (f *fakeTicker) Stop()               {}

// -------------------------
// Suite
// -------------------------

type ServiceSuite struct {
	suite.Suite

	repo   *mockArticleRepo
	client *mockFeedClient

	logBuf *bytes.Buffer
	logger *log.Logger

	svc *Service
}

func TestServiceSuite(t *testing.T) {
	suite.Run(t, new(ServiceSuite))
}

func (s *ServiceSuite) SetupTest() {
	s.repo = &mockArticleRepo{}
	s.client = &mockFeedClient{}

	s.logBuf = &bytes.Buffer{}
	s.logger = log.New(s.logBuf, "", 0)

	// Default: no maxPages, no maxPolls
	s.svc = NewService(s.repo, s.client, 10, -1, 0, s.logger)
}

// emptyResponse returns an empty page but with a non-zero NumPages so the
// service does not treat the first empty page as "last page" and instead
// relies on the "three empty pages" logic.
func emptyResponse() ECBResponse {
	resp := ECBResponse{}
	resp.PageInfo.NumPages = 4 // > 3 so three empty pages can be seen
	return resp
}

func nonEmptyResponse(numPages int) ECBResponse {
	resp := ECBResponse{
		Content: []ECBArticle{
			{ID: 123},
		},
	}
	resp.PageInfo.NumPages = numPages
	return resp
}

// -------------------------
// Tests
// -------------------------

// TestRunOnce_StopsAfterThreeEmptyPages stop after three consecutive empty pages
func (s *ServiceSuite) TestRunOnce_StopsAfterThreeEmptyPages() {
	resp := emptyResponse()

	s.client.
		On("FetchPage", mock.Anything, 0, 10).Return(resp, nil).Once()
	s.client.
		On("FetchPage", mock.Anything, 1, 10).Return(resp, nil).Once()
	s.client.
		On("FetchPage", mock.Anything, 2, 10).Return(resp, nil).Once()

	err := s.svc.RunOnce(context.Background())

	s.NoError(err)
	s.client.AssertExpectations(s.T())
	s.repo.AssertExpectations(s.T())

	// Actual log message (from your panic output) was:
	// "no content for 3 pages — stopping"
	// Be tolerant to exact punctuation and wording details.
	s.Contains(s.logBuf.String(), "no content for 3 pages")
}

// TestRunOnce_ContinueOnError ensure RunOnce itself doesn't fail when upsert
// returns an error. We limit maxPages to 1 so the service only ever fetches
// page 0, matching our single FetchPage expectation.
func (s *ServiceSuite) TestRunOnce_ContinueOnError() {
	// Override service with maxPages = 1 so only page 0 is ever fetched.
	s.svc = NewService(s.repo, s.client, 10, 1, 0, s.logger)

	resp := nonEmptyResponse(5)

	s.client.
		On("FetchPage", mock.Anything, 0, 10).
		Return(resp, nil).
		Once()

	s.repo.
		On("BulkUpsert", mock.Anything, mock.AnythingOfType("[]*article.Article")).
		Return(0, errors.New("db down")).
		Once()

	err := s.svc.RunOnce(context.Background())

	s.NoError(err)
	s.client.AssertExpectations(s.T())
	s.repo.AssertExpectations(s.T())

	s.Contains(s.logBuf.String(), "bulk upsert failed on page 0")
}

// TestRunOnce_MaxPages ensure maxPages config behaviour is correct
func (s *ServiceSuite) TestRunOnce_MaxPages() {
	// Override service with a maxPages of 2
	s.svc = NewService(s.repo, s.client, 10, 2, 0, s.logger)

	// Expect FetchPage for page 0 and 1 only.
	resp0 := ECBResponse{
		Content: []ECBArticle{
			{ID: 1},
		},
	}
	resp0.PageInfo.NumPages = 100

	resp1 := ECBResponse{
		Content: []ECBArticle{
			{ID: 2},
		},
	}
	resp1.PageInfo.NumPages = 100

	s.client.
		On("FetchPage", mock.Anything, 0, 10).
		Return(resp0, nil).
		Once()
	s.client.
		On("FetchPage", mock.Anything, 1, 10).
		Return(resp1, nil).
		Once()

	// RunOnce should upsert each non-empty page, so expect exactly two calls.
	s.repo.
		On("BulkUpsert", mock.Anything, mock.AnythingOfType("[]*article.Article")).
		Return(1, nil).
		Times(2)

	err := s.svc.RunOnce(context.Background())

	s.NoError(err)
	s.client.AssertExpectations(s.T())
	s.repo.AssertExpectations(s.T())

	s.Contains(s.logBuf.String(), "reached configured page limit 2")
}

// TestRunOnce_NumPages if there's fewer pages than MaxPages, stop
func (s *ServiceSuite) TestRunOnce_NumPages() {
	resp := nonEmptyResponse(1) // NumPages = 1

	s.client.
		On("FetchPage", mock.Anything, 0, 10).
		Return(resp, nil).
		Once()

	s.repo.
		On("BulkUpsert", mock.Anything, mock.AnythingOfType("[]*article.Article")).
		Return(1, nil).
		Once()

	err := s.svc.RunOnce(context.Background())

	s.NoError(err)
	s.client.AssertExpectations(s.T())
	s.repo.AssertExpectations(s.T())

	s.Contains(s.logBuf.String(), "reached reported last page 1")
}

// TestStartPolling_StopsAfterMaxPolls stop after maxPolls and call RunOnce that many times.
func (s *ServiceSuite) TestStartPolling_StopsAfterMaxPolls() {
	maxPolls := 2

	// fresh service with maxPolls set
	s.svc = NewService(s.repo, s.client, 10, 1, maxPolls, s.logger)

	// inject fake ticker
	tickCh := make(chan time.Time)
	ft := &fakeTicker{ch: tickCh}

	s.svc.newTicker = func(d time.Duration) ticker {
		return ft
	}

	resp := nonEmptyResponse(1)

	s.client.
		On("FetchPage", mock.Anything, 0, 10).
		Return(resp, nil).
		Times(maxPolls)

	s.repo.
		On("BulkUpsert", mock.Anything, mock.AnythingOfType("[]*article.Article")).
		Return(1, nil).
		Times(maxPolls)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		s.svc.StartPolling(ctx, time.Second)
	}()

	// Manually trigger ticks.
	// We send maxPolls+1 ticks in case StartPolling checks the stop condition
	// *after* receiving a tick (common off-by-one pattern):
	for i := 0; i < maxPolls+1; i++ {
		tickCh <- time.Now()
	}

	// Wait for StartPolling to return.
	wg.Wait()

	s.client.AssertExpectations(s.T())
	s.repo.AssertExpectations(s.T())
	s.Contains(s.logBuf.String(), "poller stopping after 2 polls")
}
