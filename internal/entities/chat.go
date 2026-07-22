package entities

import (
	"time"

	"github.com/gocql/gocql"
)

type RoomType string

const (
	RoomTypeDirect RoomType = "DIRECT"
	RoomTypeGroup  RoomType = "GROUP"
)

type MemberRole string

const (
	RoleAdmin  MemberRole = "ADMIN"
	RoleMember MemberRole = "MEMBER"
)

type ChatRoom struct {
	RoomID    gocql.UUID `json:"room_id"`
	RoomType  RoomType   `json:"room_type"`
	Name      string     `json:"name,omitempty"`
	CreatedBy gocql.UUID `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
}

type ChatRoomMember struct {
	RoomID   gocql.UUID `json:"room_id"`
	UserID   gocql.UUID `json:"user_id"`
	Role     MemberRole `json:"role"`
	JoinedAt time.Time  `json:"joined_at"`
}

type Message struct {
	RoomID      gocql.UUID `json:"room_id"`
	MessageID   gocql.UUID `json:"message_id"`
	SenderID    gocql.UUID `json:"sender_id"`
	ContentBody string     `json:"content_body"`
	IsRead      bool       `json:"is_read"`
	CreatedAt   time.Time  `json:"created_at"`
}

type ActiveChatByUser struct {
	UserID             gocql.UUID `json:"user_id"`
	LastActivity       time.Time  `json:"last_activity"`
	RoomID             gocql.UUID `json:"room_id"`
	LastMessagePreview string     `json:"last_message_preview"`
	UnreadCount        int        `json:"unread_count"`
	RoomName           string     `json:"room_name"`
	RoomType           string     `json:"room_type"`
}
