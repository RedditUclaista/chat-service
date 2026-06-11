package dto

import (
	"time"

	"github.com/gocql/gocql"
)

// --- RESPONSES ---

// RoomResponse es lo que devuelve el endpoint de creación y el listado
type RoomResponse struct {
	RoomID    gocql.UUID `json:"room_id"`
	RoomType  string     `json:"room_type"`
	Name      string     `json:"name,omitempty"`
	CreatedBy gocql.UUID `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
}

// RoomSummaryResponse es lo que devuelve GET /rooms (la inbox del usuario).
// Incluye la info desnormalizada de ACTIVE_CHATS_BY_USER para O(1).
type RoomSummaryResponse struct {
	RoomID             gocql.UUID `json:"room_id"`
	RoomName           string     `json:"room_name"`
	RoomType           string     `json:"room_type"`
	LastMessagePreview string     `json:"last_message_preview"`
	LastActivity       time.Time  `json:"last_activity"`
	UnreadCount        int        `json:"unread_count"`
}

// MessageResponse es cada mensaje dentro del historial paginado
type MessageResponse struct {
	MessageID   gocql.UUID `json:"message_id"`
	SenderID    gocql.UUID `json:"sender_id"`
	ContentBody string     `json:"content_body"`
	IsRead      bool       `json:"is_read"`
	CreatedAt   time.Time  `json:"created_at"`
}

// PaginatedMessagesResponse envuelve la lista de mensajes con el cursor
// para que el cliente pueda pedir la siguiente página.
type PaginatedMessagesResponse struct {
	Messages   []MessageResponse `json:"messages"`
	// NextCursor es el TimeUUID del último mensaje devuelto.
	// Si está vacío (""), no hay más páginas.
	NextCursor string            `json:"next_cursor,omitempty"`
}
