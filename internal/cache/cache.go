package cache

import (
	"context"
	"time"

	"github.com/RedditUclaista/chat-service/internal/entities"
	"github.com/gocql/gocql"
)

type ChatCache interface {
	IsMember(ctx context.Context, roomID, userID gocql.UUID) (bool, error)
	SetRoomMembers(ctx context.Context, roomID gocql.UUID, memberIDs []gocql.UUID, ttl time.Duration) error

	AddRecentMessage(ctx context.Context, msg entities.Message) error
	GetRecentMessages(ctx context.Context, roomID gocql.UUID, limit int) ([]entities.Message, error)

	GetUnreadCount(ctx context.Context, userID, roomID gocql.UUID) (int, error)
	IncrementUnreadCount(ctx context.Context, userID, roomID gocql.UUID) error
	ResetUnreadCount(ctx context.Context, userID, roomID gocql.UUID) error

	Close() error
}
