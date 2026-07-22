package database

import (
	"context"

	"github.com/RedditUclaista/chat-service/internal/entities"
	"github.com/gocql/gocql"
)

type ChatRepository interface {
	CreateRoomWithMembers(ctx context.Context, room entities.ChatRoom, members []entities.ChatRoomMember) error

	FindDirectRoomBetweenUsers(ctx context.Context, userA, userB gocql.UUID) (*entities.ChatRoom, error)

	GetActiveChatsForUser(ctx context.Context, userID gocql.UUID) ([]entities.ActiveChatByUser, error)

	UpsertActiveChatForUsers(ctx context.Context, memberIDs []gocql.UUID, chat entities.ActiveChatByUser) error

	GetMessagesByRoom(ctx context.Context, roomID gocql.UUID, cursor *gocql.UUID, limit int) ([]entities.Message, error)

	GetRoomMembers(ctx context.Context, roomID gocql.UUID) ([]entities.ChatRoomMember, error)

	GetRoomByID(ctx context.Context, roomID gocql.UUID) (*entities.ChatRoom, error)

	SaveMessage(ctx context.Context, msg entities.Message) error

	IsMember(ctx context.Context, roomID, userID gocql.UUID) (bool, error)
}
