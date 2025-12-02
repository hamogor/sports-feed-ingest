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

type mockArticleRepo struct {
	mock.Mock
}

func (m *mockArticleRepo) UpsertByExternalID(ctx context.Context, a *article.Article) (bool, error) {
	args := m.Called(ctx, a)
	return args.Bool(0), args.Error(1)
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

	s.svc = NewService(s.repo, s.client, 10, -1, 0, s.logger)
}

func emptyResponse() ECBResponse {
	return ECBResponse{}
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

	s.Contains(s.logBuf.String(), "no content for 3 pages")
}

// TestRunOnce_ContinueOnError ensure polling continues on err
func (s *ServiceSuite) TestRunOnce_ContinueOnError() {
	resp := nonEmptyResponse(5)

	s.client.
		On("FetchPage", mock.Anything, 0, 10).
		Return(resp, nil).
		Once()

	s.repo.
		On("UpsertByExternalID", mock.Anything, mock.AnythingOfType("*article.Article")).
		Return(false, errors.New("db down")).
		Once()

	err := s.svc.RunOnce(context.Background())

	s.NoError(err)
	s.client.AssertExpectations(s.T())
	s.repo.AssertExpectations(s.T())

	s.Contains(s.logBuf.String(), "failed to upsert")
}

// TestRunOnce_MaxPages ensure maxPages config behaviour is correct
func (s *ServiceSuite) TestRunOnce_MaxPages() {
	// Override service with a maxPages of 2
	s.svc = NewService(s.repo, s.client, 10, 2, 0, s.logger)

	resp := nonEmptyResponse(100) // lots of pages so only maxPages can stop it

	// Expect FetchPage for page 0 and 1 only.
	s.client.
		On("FetchPage", mock.Anything, 0, 10).
		Return(resp, nil).
		Once()
	s.client.
		On("FetchPage", mock.Anything, 1, 10).
		Return(resp, nil).
		Once()

	s.repo.
		On("UpsertByExternalID", mock.Anything, mock.AnythingOfType("*article.Article")).
		Return(true, nil)

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
		On("UpsertByExternalID", mock.Anything, mock.AnythingOfType("*article.Article")).
		Return(true, nil)

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

	var wg sync.WaitGroup
	wg.Add(maxPolls)

	s.client.
		On("FetchPage", mock.Anything, 0, 10).
		Return(resp, nil).
		Run(func(args mock.Arguments) {
			wg.Done()
		}).
		Times(maxPolls)

	s.repo.
		On("UpsertByExternalID", mock.Anything, mock.AnythingOfType("*article.Article")).
		Return(true, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.svc.StartPolling(ctx, time.Second)

	// Manually trigger exactly maxPolls ticks
	tickCh <- time.Now()
	tickCh <- time.Now()

	// Wait until both polls have happened
	wg.Wait()

	s.client.AssertExpectations(s.T())
	s.repo.AssertExpectations(s.T())
	s.Contains(s.logBuf.String(), "poller stopping after 2 polls")
}
