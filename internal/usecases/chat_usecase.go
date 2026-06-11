package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"
	"github.com/RedditUclaista/chat-service/internal/database"
	"github.com/RedditUclaista/chat-service/internal/dto"
	"github.com/RedditUclaista/chat-service/internal/entities"
)

// ChatUseCase contiene toda la lógica de negocio del chat.
// No sabe nada de HTTP ni de ScyllaDB. Solo habla con la interfaz del repositorio.
type ChatUseCase struct {
	repo database.ChatRepository
}

// NewChatUseCase es el constructor. Recibe la dependencia por interfaz (DI).
func NewChatUseCase(repo database.ChatRepository) *ChatUseCase {
	return &ChatUseCase{repo: repo}
}

// -------------------------------------------------------------------------
// CREAR SALA
// -------------------------------------------------------------------------

// CreateRoom orquesta la creación de una sala de chat.
// Aquí vive la regla de negocio más importante: no duplicar salas DIRECT.
func (uc *ChatUseCase) CreateRoom(ctx context.Context, creatorID gocql.UUID, req dto.CreateRoomRequest) (*dto.RoomResponse, error) {
	roomType := entities.RoomType(req.RoomType)

	// --- Regla de negocio: no duplicar chats DIRECT ---
	if roomType == entities.RoomTypeDirect {
		if len(req.MemberIDs) != 1 {
			return nil, fmt.Errorf("un chat DIRECT debe tener exactamente 1 miembro adicional, recibidos: %d", len(req.MemberIDs))
		}

		otherUserID := req.MemberIDs[0]
		existingRoom, err := uc.repo.FindDirectRoomBetweenUsers(ctx, creatorID, otherUserID)
		if err != nil {
			return nil, fmt.Errorf("verificando sala DIRECT existente: %w", err)
		}
		if existingRoom != nil {
			// Ya existe. Devolvemos la sala existente en lugar de crear una nueva.
			// Esto es más amigable que devolver un error 409.
			return &dto.RoomResponse{
				RoomID:    existingRoom.RoomID,
				RoomType:  string(existingRoom.RoomType),
				Name:      existingRoom.Name,
				CreatedBy: existingRoom.CreatedBy,
				CreatedAt: existingRoom.CreatedAt,
			}, nil
		}
	}

	// --- Construir la entidad de la sala ---
	now := time.Now().UTC()
	room := entities.ChatRoom{
		RoomID:    gocql.TimeUUID(), // TimeUUID para ordenamiento cronológico
		RoomType:  roomType,
		Name:      req.Name,
		CreatedBy: creatorID,
		CreatedAt: now,
	}

	// --- Construir la lista de miembros ---
	// El creador siempre es ADMIN
	members := []entities.ChatRoomMember{
		{
			RoomID:   room.RoomID,
			UserID:   creatorID,
			Role:     entities.RoleAdmin,
			JoinedAt: now,
		},
	}
	// El resto son MEMBER
	for _, uid := range req.MemberIDs {
		members = append(members, entities.ChatRoomMember{
			RoomID:   room.RoomID,
			UserID:   uid,
			Role:     entities.RoleMember,
			JoinedAt: now,
		})
	}

	// --- Persistir con BATCH (sala + miembros atómicamente) ---
	if err := uc.repo.CreateRoomWithMembers(ctx, room, members); err != nil {
		return nil, fmt.Errorf("creando sala en ScyllaDB: %w", err)
	}

	// --- Inicializar ACTIVE_CHATS_BY_USER para todos los miembros ---
	// Esto garantiza que la sala aparezca en la inbox de cada miembro desde el inicio.
	allMemberIDs := make([]gocql.UUID, 0, len(members))
	for _, m := range members {
		allMemberIDs = append(allMemberIDs, m.UserID)
	}

	activeChat := entities.ActiveChatByUser{
		LastActivity:       now,
		RoomID:             room.RoomID,
		LastMessagePreview: "", // Vacío: aún no hay mensajes
		UnreadCount:        0,
		RoomName:           room.Name,
	}
	// Ignoramos el error aquí intencionalmente: si falla, no es crítico para
	// la creación. Se puede recuperar cuando llegue el primer mensaje.
	_ = uc.repo.UpsertActiveChatForUsers(ctx, allMemberIDs, activeChat)

	return &dto.RoomResponse{
		RoomID:    room.RoomID,
		RoomType:  string(room.RoomType),
		Name:      room.Name,
		CreatedBy: room.CreatedBy,
		CreatedAt: room.CreatedAt,
	}, nil
}

// -------------------------------------------------------------------------
// OBTENER SALAS DEL USUARIO (INBOX)
// -------------------------------------------------------------------------

// GetUserRooms retorna la inbox del usuario autenticado.
// Lee desde ACTIVE_CHATS_BY_USER (O(1)) — ya está desnormalizado.
func (uc *ChatUseCase) GetUserRooms(ctx context.Context, userID gocql.UUID) ([]dto.RoomSummaryResponse, error) {
	chats, err := uc.repo.GetActiveChatsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("obteniendo chats activos para usuario %s: %w", userID, err)
	}

	response := make([]dto.RoomSummaryResponse, 0, len(chats))
	for _, c := range chats {
		response = append(response, dto.RoomSummaryResponse{
			RoomID:             c.RoomID,
			RoomName:           c.RoomName,
			LastMessagePreview: c.LastMessagePreview,
			LastActivity:       c.LastActivity,
			UnreadCount:        c.UnreadCount,
		})
	}

	return response, nil
}

// -------------------------------------------------------------------------
// HISTORIAL DE MENSAJES PAGINADO
// -------------------------------------------------------------------------

// GetRoomMessages retorna el historial paginado de mensajes de una sala.
// Implementa cursor-based pagination usando el TimeUUID del mensaje como cursor.
func (uc *ChatUseCase) GetRoomMessages(ctx context.Context, userID, roomID gocql.UUID, req dto.GetMessagesRequest) (*dto.PaginatedMessagesResponse, error) {
	// --- Verificar que el usuario es miembro de la sala ---
	// Seguridad básica: un usuario no puede leer mensajes de una sala en la que no está.
	members, err := uc.repo.GetRoomMembers(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("verificando membresía: %w", err)
	}

	isMember := false
	for _, m := range members {
		if m.UserID == userID {
			isMember = true
			break
		}
	}
	if !isMember {
		return nil, fmt.Errorf("acceso denegado: el usuario no es miembro de esta sala")
	}

	// --- Parsear el cursor si viene ---
	var cursor *gocql.UUID
	if req.Cursor != "" {
		parsed, err := gocql.ParseUUID(req.Cursor)
		if err != nil {
			return nil, fmt.Errorf("cursor inválido: %w", err)
		}
		cursor = &parsed
	}

	// --- Traer los mensajes ---
	messages, err := uc.repo.GetMessagesByRoom(ctx, roomID, cursor, req.Limit)
	if err != nil {
		return nil, fmt.Errorf("obteniendo mensajes de sala %s: %w", roomID, err)
	}

	// --- Mapear entidades a DTOs ---
	msgResponses := make([]dto.MessageResponse, 0, len(messages))
	for _, m := range messages {
		msgResponses = append(msgResponses, dto.MessageResponse{
			MessageID:   m.MessageID,
			SenderID:    m.SenderID,
			ContentBody: m.ContentBody,
			IsRead:      m.IsRead,
			CreatedAt:   m.CreatedAt,
		})
	}

	// --- Calcular el next_cursor ---
	// Si recibimos exactamente `limit` mensajes, probablemente hay más.
	// El cursor es el message_id del último mensaje de esta página.
	var nextCursor string
	if len(messages) == req.Limit {
		nextCursor = messages[len(messages)-1].MessageID.String()
	}

	return &dto.PaginatedMessagesResponse{
		Messages:   msgResponses,
		NextCursor: nextCursor,
	}, nil
}
