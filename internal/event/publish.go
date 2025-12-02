package event

import (
	"context"
	"cortex-task/internal/article"
	"encoding/json"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type ArticleUpdatedMessage struct {
	Event     string          `json:"event"`
	Timestamp time.Time       `json:"timestamp"`
	Article   article.Article `json:"article"`
}

type PublishingChannel interface {
	PublishWithContext(
		ctx context.Context,
		exchange, key string,
		mandatory, immediate bool,
		msg amqp.Publishing,
	) error
	Close() error
}

type RabbitPublisher struct {
	conn       *amqp.Connection
	ch         PublishingChannel
	exchange   string
	routingKey string
	logger     *log.Logger
}

func NewRabbitPublisher(uri, exchange, routingKey string, logger *log.Logger) (*RabbitPublisher, error) {
	conn, err := amqp.Dial(uri)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq connection failed: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq channel creation failed: %w", err)
	}

	if err := ch.ExchangeDeclare(
		exchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("exchange declare failed: %w", err)
	}

	return &RabbitPublisher{
		conn:       conn,
		ch:         ch,
		exchange:   exchange,
		routingKey: routingKey,
		logger:     logger,
	}, nil
}

func (p *RabbitPublisher) Close() {
	if p.ch != nil {
		_ = p.ch.Close()
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
}

func (p *RabbitPublisher) PublishArticleUpdated(ctx context.Context, a *article.Article) error {
	body, err := json.Marshal(ArticleUpdatedMessage{
		Event:     "article.updated",
		Timestamp: time.Now().UTC(),
		Article:   *a,
	})
	if err != nil {
		return err
	}

	return p.ch.PublishWithContext(
		ctx,
		p.exchange,
		p.routingKey,
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         body,
		},
	)
}
