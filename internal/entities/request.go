package dto

import "github.com/gocql/gocql"

// --- REQUESTS ---

// CreateRoomRequest es el body que espera el endpoint POST /rooms
type CreateRoomRequest struct {
	// Tipo de sala: "DIRECT" o "GROUP"
	RoomType string `json:"room_type" validate:"required,oneof=DIRECT GROUP"`

	// Para salas GROUP, es obligatorio. Para DIRECT, se ignora.
	Name string `json:"name,omitempty"`

	// IDs de los miembros iniciales.
	// Para DIRECT: debe contener exactamente 1 user_id (el otro participante).
	// Para GROUP:  debe contener al menos 1 user_id.
	MemberIDs []gocql.UUID `json:"member_ids" validate:"required,min=1"`
}

// GetMessagesRequest agrupa los query params del endpoint GET /rooms/:id/messages
type GetMessagesRequest struct {
	// Cursor basado en TimeUUID para la paginación.
	// Si está vacío, trae la página más reciente.
	Cursor string `query:"cursor"`

	// Cuántos mensajes traer por página. Default: 30, Max: 100
	Limit int `query:"limit"`
}
