package bus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/RedditUclaista/chat-service/internal/entities"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

func NewConsumer(conn *amqp.Connection) (*Consumer, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("bus: abriendo canal consumer: %w", err)
	}

	if err := ch.ExchangeDeclare(
		exchangeName, exchangeType, true, false, false, false, nil,
	); err != nil {
		ch.Close()
		return nil, fmt.Errorf("bus: declarando exchange: %w", err)
	}

	return &Consumer{conn: conn, channel: ch}, nil
}

func (c *Consumer) ConsumeFanout(ctx context.Context, instanceID string, handler func(context.Context, entities.Message) error) error {
	q, err := c.channel.QueueDeclare(
		fmt.Sprintf("chat.fanout.%s", instanceID),
		false, true, true, false, nil,
	)
	if err != nil {
		return fmt.Errorf("bus: declarando cola fanout: %w", err)
	}

	if err := c.channel.QueueBind(q.Name, "#", exchangeName, false, nil); err != nil {
		return fmt.Errorf("bus: bind cola fanout: %w", err)
	}

	msgs, err := c.channel.Consume(q.Name, "", true, true, false, false, nil)
	if err != nil {
		return fmt.Errorf("bus: consume fanout: %w", err)
	}

	slog.Info("consumer fanout iniciado", "queue", q.Name, "instance", instanceID)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-msgs:
			if !ok {
				return fmt.Errorf("bus: canal fanout cerrado")
			}
			var msg entities.Message
			if err := json.Unmarshal(d.Body, &msg); err != nil {
				slog.Warn("bus: mensaje fanout invalido", "error", err)
				continue
			}
			if err := handler(ctx, msg); err != nil {
				slog.Error("bus: handler fanout fallo", "error", err)
			}
		}
	}
}

func (c *Consumer) ConsumePersistence(ctx context.Context, handler func(context.Context, entities.Message) error) error {
	q, err := c.channel.QueueDeclare(
		"chat.persistence",
		true, false, false, false, nil,
	)
	if err != nil {
		return fmt.Errorf("bus: declarando cola persistence: %w", err)
	}

	if err := c.channel.QueueBind(q.Name, "#", exchangeName, false, nil); err != nil {
		return fmt.Errorf("bus: bind cola persistence: %w", err)
	}

	_ = c.channel.Qos(10, 0, false)

	msgs, err := c.channel.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("bus: consume persistence: %w", err)
	}

	slog.Info("consumer persistence iniciado", "queue", q.Name)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-msgs:
			if !ok {
				return fmt.Errorf("bus: canal persistence cerrado")
			}
			var msg entities.Message
			if err := json.Unmarshal(d.Body, &msg); err != nil {
				slog.Warn("bus: mensaje persistence invalido", "error", err)
				d.Nack(false, false)
				continue
			}
			if err := handler(ctx, msg); err != nil {
				slog.Error("bus: handler persistence fallo", "error", err)
				d.Nack(false, true)
				continue
			}
			d.Ack(false)
		}
	}
}

func (c *Consumer) Close() error {
	return c.channel.Close()
}
