package messaging

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Publisher struct {
	conn  *amqp.Connection
	ch    *amqp.Channel
	queue string
	mu    sync.Mutex
}

func NewPublisher(url string, queue string) (*Publisher, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, err
	}
	if _, err := ch.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, err
	}
	return &Publisher{conn: conn, ch: ch, queue: queue}, nil
}

func (p *Publisher) Publish(ctx context.Context, messageID string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ch.PublishWithContext(ctx, "", p.queue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    messageID,
		Timestamp:    time.Now().UTC(),
		Body:         body,
	})
}

func (p *Publisher) Close() {
	if p.ch != nil {
		_ = p.ch.Close()
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
}
