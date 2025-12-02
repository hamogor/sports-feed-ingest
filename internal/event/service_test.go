package event

import (
	"context"
	"cortex-task/internal/article"
	"errors"
	"io"
	"log"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// -------------------------
// Mock AMQP channel
// -------------------------

type MockAMQPChannel struct {
	mock.Mock
}

func (m *MockAMQPChannel) PublishWithContext(
	ctx context.Context,
	exchange, key string,
	mandatory, immediate bool,
	msg amqp.Publishing,
) error {
	args := m.Called(ctx, exchange, key, mandatory, immediate, msg)
	return args.Error(0)
}

func (m *MockAMQPChannel) Close() error { return nil } // unused, but needed

// -------------------------
// Helper
// -------------------------

func newTestPublisher(mockCh *MockAMQPChannel) *RabbitPublisher {
	return &RabbitPublisher{
		conn:       nil,
		ch:         mockCh,
		exchange:   "cms.sync",
		routingKey: "article.updated",
		logger:     log.New(io.Discard, "", 0),
	}
}

// -------------------------
// Tests
// -------------------------

func TestPublishArticleUpdated_PublishesCorrectly(t *testing.T) {
	mockCh := &MockAMQPChannel{}
	pub := newTestPublisher(mockCh)

	art := &article.Article{
		ExternalID: 1001,
		Title:      "Sample",
	}

	mockCh.
		On("PublishWithContext",
			mock.Anything,
			"cms.sync",
			"article.updated",
			false,
			false,
			mock.AnythingOfType("amqp.Publishing"),
		).
		Return(nil).
		Once()

	err := pub.PublishArticleUpdated(context.Background(), art)
	require.NoError(t, err)

	mockCh.AssertExpectations(t)
}

func TestPublishArticleUpdated_JSONContainsArticle(t *testing.T) {
	mockCh := &MockAMQPChannel{}
	pub := newTestPublisher(mockCh)

	art := &article.Article{
		ExternalID: 1234,
		Title:      "Test Title",
	}

	var capturedMsg amqp.Publishing

	mockCh.
		On("PublishWithContext",
			mock.Anything,
			"cms.sync",
			"article.updated",
			false,
			false,
			mock.AnythingOfType("amqp.Publishing"),
		).
		Return(nil).
		Run(func(args mock.Arguments) {
			capturedMsg = args.Get(5).(amqp.Publishing)
		})

	err := pub.PublishArticleUpdated(context.Background(), art)
	require.NoError(t, err)

	body := string(capturedMsg.Body)

	assert.Contains(t, body, `"event":"article.updated"`)
	assert.Contains(t, body, `"externalId":1234`)
	assert.Contains(t, body, `"Test Title"`)
}

func TestPublishArticleUpdated_ErrorBubbles(t *testing.T) {
	mockCh := &MockAMQPChannel{}
	pub := newTestPublisher(mockCh)

	publishErr := errors.New("boom")

	mockCh.
		On("PublishWithContext",
			mock.Anything,
			mock.Anything,
			mock.Anything,
			mock.Anything,
			mock.Anything,
			mock.Anything,
		).
		Return(publishErr)

	err := pub.PublishArticleUpdated(context.Background(), &article.Article{})
	require.Error(t, err)
	require.Equal(t, publishErr, err)
}

func TestPublishArticleUpdated_ContextCancel(t *testing.T) {
	mockCh := &MockAMQPChannel{}
	pub := newTestPublisher(mockCh)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := pub.PublishArticleUpdated(ctx, &article.Article{})
	require.Error(t, err)
	require.Equal(t, context.Canceled, err)
}
