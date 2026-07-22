package dto

import (
	"time"

	"github.com/gocql/gocql"
)

type RoomResponse struct {
	RoomID    gocql.UUID `json:"room_id"`
	RoomType  string     `json:"room_type"`
	Name      string     `json:"name,omitempty"`
	CreatedBy gocql.UUID `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
}

type RoomSummaryResponse struct {
	RoomID             gocql.UUID `json:"room_id"`
	RoomName           string     `json:"room_name"`
	RoomType           string     `json:"room_type"`
	LastMessagePreview string     `json:"last_message_preview"`
	LastActivity       time.Time  `json:"last_activity"`
	UnreadCount        int        `json:"unread_count"`
}

type MessageResponse struct {
	MessageID   gocql.UUID `json:"message_id"`
	RoomID      gocql.UUID `json:"room_id"`
	SenderID    gocql.UUID `json:"sender_id"`
	ContentBody string     `json:"content_body"`
	IsRead      bool       `json:"is_read"`
	CreatedAt   time.Time  `json:"created_at"`
}

type PaginatedMessagesResponse struct {
	Messages   []MessageResponse `json:"messages"`
	NextCursor string            `json:"next_cursor,omitempty"`
}
