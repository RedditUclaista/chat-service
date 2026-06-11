package entities

import (
	"time"

	"github.com/gocql/gocql"
)

// RoomType define los tipos de sala posibles
type RoomType string

const (
	RoomTypeDirect RoomType = "DIRECT"
	RoomTypeGroup  RoomType = "GROUP"
)

// MemberRole define los roles dentro de una sala
type MemberRole string

const (
	RoleAdmin  MemberRole = "ADMIN"
	RoleMember MemberRole = "MEMBER"
)

// ChatRoom representa la tabla CHAT_ROOMS en ScyllaDB
type ChatRoom struct {
	RoomID    gocql.UUID `json:"room_id"`
	RoomType  RoomType   `json:"room_type"`
	Name      string     `json:"name,omitempty"` // Nullable para chats DIRECT
	CreatedBy gocql.UUID `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
}

// ChatRoomMember representa la tabla CHAT_ROOM_MEMBERS en ScyllaDB
type ChatRoomMember struct {
	RoomID   gocql.UUID `json:"room_id"`
	UserID   gocql.UUID `json:"user_id"`
	Role     MemberRole `json:"role"`
	JoinedAt time.Time  `json:"joined_at"`
}

// Message representa la tabla MESSAGES_BY_ROOM en ScyllaDB
type Message struct {
	RoomID      gocql.UUID `json:"room_id"`
	MessageID   gocql.UUID `json:"message_id"` // TimeUUID
	SenderID    gocql.UUID `json:"sender_id"`
	ContentBody string     `json:"content_body"`
	IsRead      bool       `json:"is_read"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ActiveChatByUser representa la tabla ACTIVE_CHATS_BY_USER en ScyllaDB.
// Esta tabla está desnormalizada para lograr lecturas O(1) en la inbox del usuario.
type ActiveChatByUser struct {
	UserID             gocql.UUID `json:"user_id"`
	LastActivity       time.Time  `json:"last_activity"`
	RoomID             gocql.UUID `json:"room_id"`
	LastMessagePreview string     `json:"last_message_preview"`
	UnreadCount        int        `json:"unread_count"`
	RoomName           string     `json:"room_name"` // Desnormalizado para lectura O(1)
}
