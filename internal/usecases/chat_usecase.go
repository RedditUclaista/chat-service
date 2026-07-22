package usecases

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/RedditUclaista/chat-service/internal/bus"
	"github.com/RedditUclaista/chat-service/internal/cache"
	"github.com/RedditUclaista/chat-service/internal/database"
	"github.com/RedditUclaista/chat-service/internal/dto"
	"github.com/RedditUclaista/chat-service/internal/entities"
	"github.com/RedditUclaista/chat-service/internal/errs"
	"github.com/RedditUclaista/chat-service/internal/hub"
	"github.com/gocql/gocql"
)

type ChatUseCase struct {
	repo      database.ChatRepository
	cache     cache.ChatCache
	publisher *bus.Publisher
	hub       *hub.Hub
}

func NewChatUseCase(
	repo database.ChatRepository,
	cache cache.ChatCache,
	publisher *bus.Publisher,
	hub *hub.Hub,
) *ChatUseCase {
	return &ChatUseCase{
		repo:      repo,
		cache:     cache,
		publisher: publisher,
		hub:       hub,
	}
}

func (uc *ChatUseCase) CreateRoom(ctx context.Context, creatorID gocql.UUID, req dto.CreateRoomRequest) (*dto.RoomResponse, error) {
	roomType := entities.RoomType(req.RoomType)

	if roomType == entities.RoomTypeDirect {
		if len(req.MemberIDs) != 1 {
			return nil, errs.ErrInvalidRoomType
		}

		otherUserID := req.MemberIDs[0]
		existingRoom, err := uc.repo.FindDirectRoomBetweenUsers(ctx, creatorID, otherUserID)
		if err != nil {
			return nil, err
		}
		if existingRoom != nil {
			return &dto.RoomResponse{
				RoomID:    existingRoom.RoomID,
				RoomType:  string(existingRoom.RoomType),
				Name:      existingRoom.Name,
				CreatedBy: existingRoom.CreatedBy,
				CreatedAt: existingRoom.CreatedAt,
			}, nil
		}
	}

	now := time.Now().UTC()
	room := entities.ChatRoom{
		RoomID:    gocql.TimeUUID(),
		RoomType:  roomType,
		Name:      req.Name,
		CreatedBy: creatorID,
		CreatedAt: now,
	}

	members := []entities.ChatRoomMember{
		{
			RoomID:   room.RoomID,
			UserID:   creatorID,
			Role:     entities.RoleAdmin,
			JoinedAt: now,
		},
	}
	for _, uid := range req.MemberIDs {
		members = append(members, entities.ChatRoomMember{
			RoomID:   room.RoomID,
			UserID:   uid,
			Role:     entities.RoleMember,
			JoinedAt: now,
		})
	}

	if err := uc.repo.CreateRoomWithMembers(ctx, room, members); err != nil {
		return nil, err
	}

	allMemberIDs := make([]gocql.UUID, 0, len(members))
	for _, m := range members {
		allMemberIDs = append(allMemberIDs, m.UserID)
	}

	activeChat := entities.ActiveChatByUser{
		LastActivity:       now,
		RoomID:             room.RoomID,
		LastMessagePreview: "",
		UnreadCount:        0,
		RoomName:           room.Name,
		RoomType:           string(room.RoomType),
	}
	if err := uc.repo.UpsertActiveChatForUsers(ctx, allMemberIDs, activeChat); err != nil {
		slog.Warn("repo: fallo al inicializar active chats", "error", err)
	}

	memberIDs := make([]gocql.UUID, len(members))
	for i, m := range members {
		memberIDs[i] = m.UserID
	}
	if err := uc.cache.SetRoomMembers(ctx, room.RoomID, memberIDs, 5*time.Minute); err != nil {
		slog.Warn("cache: fallo al cachear miembros", "error", err)
	}

	return &dto.RoomResponse{
		RoomID:    room.RoomID,
		RoomType:  string(room.RoomType),
		Name:      room.Name,
		CreatedBy: room.CreatedBy,
		CreatedAt: room.CreatedAt,
	}, nil
}

func (uc *ChatUseCase) GetUserRooms(ctx context.Context, userID gocql.UUID) ([]dto.RoomSummaryResponse, error) {
	chats, err := uc.repo.GetActiveChatsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	response := make([]dto.RoomSummaryResponse, 0, len(chats))
	for _, c := range chats {
		response = append(response, dto.RoomSummaryResponse{
			RoomID:             c.RoomID,
			RoomName:           c.RoomName,
			RoomType:           c.RoomType,
			LastMessagePreview: c.LastMessagePreview,
			LastActivity:       c.LastActivity,
			UnreadCount:        c.UnreadCount,
		})
	}

	return response, nil
}

func (uc *ChatUseCase) GetRoomMessages(ctx context.Context, userID, roomID gocql.UUID, req dto.GetMessagesRequest) (*dto.PaginatedMessagesResponse, error) {
	isMember, err := uc.checkMembership(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, errs.ErrNotMember
	}

	var cursor *gocql.UUID
	if req.Cursor != "" {
		parsed, err := gocql.ParseUUID(req.Cursor)
		if err != nil {
			return nil, errs.ErrInvalidCursor
		}
		cursor = &parsed
	}

	messages, err := uc.repo.GetMessagesByRoom(ctx, roomID, cursor, req.GetLimit())
	if err != nil {
		return nil, err
	}

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

	var nextCursor string
	if len(messages) == req.GetLimit() {
		nextCursor = messages[len(messages)-1].MessageID.String()
	}

	return &dto.PaginatedMessagesResponse{
		Messages:   msgResponses,
		NextCursor: nextCursor,
	}, nil
}

func (uc *ChatUseCase) SendMessage(ctx context.Context, senderID, roomID gocql.UUID, body string) (*dto.MessageResponse, error) {
	isMember, err := uc.checkMembership(ctx, roomID, senderID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, errs.ErrNotMember
	}

	now := time.Now().UTC()
	msg := entities.Message{
		RoomID:      roomID,
		MessageID:   gocql.TimeUUID(),
		SenderID:    senderID,
		ContentBody: body,
		IsRead:      false,
		CreatedAt:   now,
	}

	if err := uc.cache.AddRecentMessage(ctx, msg); err != nil {
		slog.Warn("cache: fallo al agregar mensaje reciente", "error", err)
	}

	go func() {
		bgCtx := context.Background()
		if err := uc.publisher.PublishMessage(bgCtx, msg); err != nil {
			slog.Error("bus: fallo al publicar mensaje", "message_id", msg.MessageID, "error", err)
		}
	}()

	return &dto.MessageResponse{
		MessageID:   msg.MessageID,
		RoomID:      msg.RoomID,
		SenderID:    msg.SenderID,
		ContentBody: msg.ContentBody,
		IsRead:      false,
		CreatedAt:   msg.CreatedAt,
	}, nil
}

func (uc *ChatUseCase) HandleMessageFanout(ctx context.Context, msg entities.Message) error {
	members, err := uc.getMemberIDs(ctx, msg.RoomID)
	if err != nil {
		return err
	}

	envPayload := dto.MessageSentPayload{
		MessageID: msg.MessageID.String(),
		RoomID:    msg.RoomID.String(),
		SenderID:  msg.SenderID.String(),
		Body:      msg.ContentBody,
		CreatedAt: msg.CreatedAt.Format(time.RFC3339Nano),
	}
	envelope, _ := dto.NewEnvelope(dto.EventNewMessage, envPayload)
	data, _ := json.Marshal(envelope)

	uc.hub.BroadcastToRoom(members, data, msg.SenderID)
	return nil
}

func (uc *ChatUseCase) HandleMessagePersistence(ctx context.Context, msg entities.Message) error {
	if err := uc.repo.SaveMessage(ctx, msg); err != nil {
		return err
	}

	members, err := uc.repo.GetRoomMembers(ctx, msg.RoomID)
	if err != nil {
		return err
	}

	memberIDs := make([]gocql.UUID, len(members))
	roomName := ""
	roomType := ""
	for i, m := range members {
		memberIDs[i] = m.UserID
		if m.UserID == msg.SenderID {
			continue
		}
	}

	room, err := uc.repo.GetRoomByID(ctx, msg.RoomID)
	if err == nil && room != nil {
		roomName = room.Name
		roomType = string(room.RoomType)
	}

	preview := truncate(msg.ContentBody, 100)
	activeChat := entities.ActiveChatByUser{
		LastActivity:       msg.CreatedAt,
		RoomID:             msg.RoomID,
		LastMessagePreview: preview,
		RoomName:           roomName,
		RoomType:           roomType,
	}

	if err := uc.repo.UpsertActiveChatForUsers(ctx, memberIDs, activeChat); err != nil {
		slog.Warn("repo: fallo al actualizar active chats", "error", err)
	}

	for _, uid := range memberIDs {
		if uid != msg.SenderID {
			if err := uc.cache.IncrementUnreadCount(ctx, uid, msg.RoomID); err != nil {
				slog.Warn("cache: fallo al incrementar unread", "error", err)
			}
		}
	}

	return nil
}

func (uc *ChatUseCase) GetRoomMembers(ctx context.Context, userID, roomID gocql.UUID) ([]gocql.UUID, error) {
	isMember, err := uc.checkMembership(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, errs.ErrNotMember
	}
	return uc.getMemberIDs(ctx, roomID)
}

func (uc *ChatUseCase) checkMembership(ctx context.Context, roomID, userID gocql.UUID) (bool, error) {
	ok, err := uc.cache.IsMember(ctx, roomID, userID)
	if err == nil && ok {
		return true, nil
	}

	isMember, err := uc.repo.IsMember(ctx, roomID, userID)
	if err != nil {
		return false, err
	}
	if !isMember {
		return false, nil
	}

	go func() {
		members, err := uc.repo.GetRoomMembers(ctx, roomID)
		if err != nil {
			return
		}
		memberIDs := make([]gocql.UUID, len(members))
		for i, m := range members {
			memberIDs[i] = m.UserID
		}
		_ = uc.cache.SetRoomMembers(ctx, roomID, memberIDs, 5*time.Minute)
	}()

	return true, nil
}

func (uc *ChatUseCase) getMemberIDs(ctx context.Context, roomID gocql.UUID) ([]gocql.UUID, error) {
	members, err := uc.repo.GetRoomMembers(ctx, roomID)
	if err != nil {
		return nil, err
	}

	ids := make([]gocql.UUID, len(members))
	for i, m := range members {
		ids[i] = m.UserID
	}

	go func() {
		_ = uc.cache.SetRoomMembers(ctx, roomID, ids, 5*time.Minute)
	}()

	return ids, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
