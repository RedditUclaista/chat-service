package database

import (
	"context"
	"fmt"
	"time"

	"github.com/RedditUclaista/chat-service/internal/entities"
	"github.com/gocql/gocql"
)

type scyllaRepository struct {
	session *gocql.Session
}

func NewScyllaRepository(session *gocql.Session) ChatRepository {
	return &scyllaRepository{session: session}
}

func (r *scyllaRepository) CreateRoomWithMembers(ctx context.Context, room entities.ChatRoom, members []entities.ChatRoomMember) error {
	batch := r.session.NewBatch(gocql.UnloggedBatch).WithContext(ctx)

	batch.Query(
		`INSERT INTO chat_rooms (room_id, room_type, name, created_by, created_at) VALUES (?, ?, ?, ?, ?)`,
		room.RoomID, string(room.RoomType), room.Name, room.CreatedBy, room.CreatedAt,
	)

	for _, m := range members {
		batch.Query(
			`INSERT INTO chat_room_members (room_id, user_id, role, joined_at) VALUES (?, ?, ?, ?)`,
			m.RoomID, m.UserID, string(m.Role), m.JoinedAt,
		)
	}

	return r.session.ExecuteBatch(batch)
}

func (r *scyllaRepository) FindDirectRoomBetweenUsers(ctx context.Context, userA, userB gocql.UUID) (*entities.ChatRoom, error) {
	iter := r.session.Query(
		`SELECT room_id FROM chat_room_members WHERE user_id = ? ALLOW FILTERING`,
		userA,
	).WithContext(ctx).Iter()

	var roomIDs []gocql.UUID
	var roomID gocql.UUID
	for iter.Scan(&roomID) {
		roomIDs = append(roomIDs, roomID)
	}
	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("findDirectRoom: scan userA rooms: %w", err)
	}

	for _, rid := range roomIDs {
		room, err := r.GetRoomByID(ctx, rid)
		if err != nil || room == nil {
			continue
		}
		if room.RoomType != entities.RoomTypeDirect {
			continue
		}

		// Verificar si userB es miembro de esta sala DIRECT
		var count int
		err = r.session.Query(
			`SELECT COUNT(*) FROM chat_room_members WHERE room_id = ? AND user_id = ?`,
			rid, userB,
		).WithContext(ctx).Scan(&count)
		if err != nil {
			continue
		}
		if count > 0 {
			return room, nil // ¡Encontramos la sala!
		}
	}

	return nil, nil // No existe sala DIRECT entre estos dos usuarios
}

func (r *scyllaRepository) GetRoomByID(ctx context.Context, roomID gocql.UUID) (*entities.ChatRoom, error) {
	room := &entities.ChatRoom{}
	err := r.session.Query(
		`SELECT room_id, room_type, name, created_by, created_at FROM chat_rooms WHERE room_id = ?`,
		roomID,
	).WithContext(ctx).Scan(
		&room.RoomID, &room.RoomType, &room.Name, &room.CreatedBy, &room.CreatedAt,
	)
	if err == gocql.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getRoomByID %s: %w", roomID, err)
	}
	return room, nil
}

func (r *scyllaRepository) GetRoomMembers(ctx context.Context, roomID gocql.UUID) ([]entities.ChatRoomMember, error) {
	// Esta query es eficiente: room_id es la Partition Key de CHAT_ROOM_MEMBERS,
	// así que Scylla va directamente al nodo correcto sin scatter/gather.
	iter := r.session.Query(
		`SELECT room_id, user_id, role, joined_at FROM chat_room_members WHERE room_id = ?`,
		roomID,
	).WithContext(ctx).Iter()

	var members []entities.ChatRoomMember
	var m entities.ChatRoomMember
	for iter.Scan(&m.RoomID, &m.UserID, &m.Role, &m.JoinedAt) {
		members = append(members, m)
	}
	return members, iter.Close()
}

// -------------------------------------------------------------------------
// INBOX DEL USUARIO
// -------------------------------------------------------------------------

func (r *scyllaRepository) GetActiveChatsForUser(ctx context.Context, userID gocql.UUID) ([]entities.ActiveChatByUser, error) {
	// Query O(1): user_id es la Partition Key de ACTIVE_CHATS_BY_USER.
	// Scylla devuelve las filas ordenadas por last_activity DESC gracias
	// a la Clustering Key definida en el schema.
	iter := r.session.Query(
		`SELECT user_id, last_activity, room_id, last_message_preview, unread_count, room_name
		 FROM active_chats_by_user
		 WHERE user_id = ?`,
		userID,
	).WithContext(ctx).Iter()

	var chats []entities.ActiveChatByUser
	var c entities.ActiveChatByUser
	for iter.Scan(&c.UserID, &c.LastActivity, &c.RoomID, &c.LastMessagePreview, &c.UnreadCount, &c.RoomName) {
		chats = append(chats, c)
	}
	return chats, iter.Close()
}

func (r *scyllaRepository) UpsertActiveChatForUsers(ctx context.Context, memberIDs []gocql.UUID, chat entities.ActiveChatByUser) error {
	// Actualizamos la fila de ACTIVE_CHATS_BY_USER para cada miembro de la sala.
	// Usamos un BATCH para hacerlo en una sola ida de red.
	// La nota de tu diagrama dice: "Updated concurrently on new message" — esto es eso.
	batch := r.session.NewBatch(gocql.UnloggedBatch).WithContext(ctx)

	for _, uid := range memberIDs {
		batch.Query(
			`INSERT INTO active_chats_by_user
			 (user_id, last_activity, room_id, last_message_preview, unread_count, room_name)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			uid,
			chat.LastActivity,
			chat.RoomID,
			chat.LastMessagePreview,
			chat.UnreadCount,
			chat.RoomName,
		)
	}

	return r.session.ExecuteBatch(batch)
}

// -------------------------------------------------------------------------
// MENSAJES
// -------------------------------------------------------------------------

func (r *scyllaRepository) GetMessagesByRoom(ctx context.Context, roomID gocql.UUID, cursor *gocql.UUID, limit int) ([]entities.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 30 // valor por defecto seguro
	}

	var query *gocql.Query

	if cursor == nil {
		// Sin cursor: traer los mensajes más recientes.
		// La Clustering Key DESC ya los ordena de más nuevo a más viejo.
		query = r.session.Query(
			`SELECT room_id, message_id, sender_id, content_body, is_read, created_at
			 FROM messages_by_room
			 WHERE room_id = ?
			 LIMIT ?`,
			roomID, limit,
		).WithContext(ctx)
	} else {
		// Con cursor: traer mensajes más viejos que el cursor (paginación hacia atrás).
		// TOKEN y TimeUUID en Scylla nos permiten hacer esto eficientemente.
		query = r.session.Query(
			`SELECT room_id, message_id, sender_id, content_body, is_read, created_at
			 FROM messages_by_room
			 WHERE room_id = ?
			 AND message_id < ?
			 LIMIT ?`,
			roomID, cursor, limit,
		).WithContext(ctx)
	}

	iter := query.Iter()
	var messages []entities.Message
	var msg entities.Message
	for iter.Scan(&msg.RoomID, &msg.MessageID, &msg.SenderID, &msg.ContentBody, &msg.IsRead, &msg.CreatedAt) {
		// Derivamos CreatedAt desde el TimeUUID para tener el timestamp preciso
		msg.CreatedAt = time.Unix(0, msg.MessageID.Time().UnixNano())
		messages = append(messages, msg)
	}
	return messages, iter.Close()
}
