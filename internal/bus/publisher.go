package bus

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/RedditUclaista/chat-service/internal/entities"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	exchangeName = "chat.messages"
	exchangeType = "topic"
)

type Publisher struct {
	channel *amqp.Channel
}

func NewPublisher(conn *amqp.Connection) (*Publisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("bus: abriendo canal: %w", err)
	}

	if err := ch.ExchangeDeclare(
		exchangeName, exchangeType, true, false, false, false, nil,
	); err != nil {
		ch.Close()
		return nil, fmt.Errorf("bus: declarando exchange: %w", err)
	}

	return &Publisher{channel: ch}, nil
}

func (p *Publisher) PublishMessage(ctx context.Context, msg entities.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	routingKey := fmt.Sprintf("room.%s", msg.RoomID.String())

	return p.channel.PublishWithContext(
		ctx, exchangeName, routingKey, false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    msg.MessageID.String(),
			Timestamp:    msg.CreatedAt,
			Body:         data,
		},
	)
}

func (p *Publisher) Close() error {
	return p.channel.Close()
}
