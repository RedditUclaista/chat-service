package websocket

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/RedditUclaista/chat-service/internal/dto"
	"github.com/RedditUclaista/chat-service/internal/hub"
	"github.com/RedditUclaista/chat-service/internal/usecases"
	"github.com/gocql/gocql"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Handler struct {
	usecase *usecases.ChatUseCase
	hub     *hub.Hub
}

func NewHandler(uc *usecases.ChatUseCase, h *hub.Hub) *Handler {
	return &Handler{usecase: uc, hub: h}
}

func (h *Handler) HandleWS(c *echo.Context) error {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "usuario no autenticado")
	}

	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		slog.Error("ws: upgrade fallo", "error", err)
		return err
	}

	h.hub.Register(userID, conn)
	slog.Info("ws: conexion registrada", "user_id", userID)

	defer func() {
		h.hub.Unregister(userID, conn)
		conn.Close()
		slog.Info("ws: conexion cerrada", "user_id", userID)
	}()

	conn.SetReadLimit(65536)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var envelope dto.Envelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			slog.Warn("ws: envelope invalido", "user_id", userID, "error", err)
			writeError(conn, "INVALID_ENVELOPE", "formato de mensaje invalido")
			continue
		}

		switch envelope.Event {
		case dto.EventNewMessage:
			h.handleNewMessage(conn, userID, envelope.Payload)
		case dto.EventTyping:
			h.handleTyping(userID, envelope.Payload)
		default:
			writeError(conn, "UNKNOWN_EVENT", "evento desconocido: "+envelope.Event)
		}
	}

	return nil
}

func (h *Handler) handleNewMessage(conn *websocket.Conn, senderID gocql.UUID, payload json.RawMessage) {
	var msgReq dto.NewMessagePayload
	if err := json.Unmarshal(payload, &msgReq); err != nil {
		writeError(conn, "INVALID_PAYLOAD", "payload de mensaje invalido")
		return
	}

	if msgReq.Body == "" || msgReq.RoomID == "" {
		writeError(conn, "VALIDATION_ERROR", "room_id y body son requeridos")
		return
	}

	roomID, err := gocql.ParseUUID(msgReq.RoomID)
	if err != nil {
		writeError(conn, "INVALID_ROOM_ID", "room_id debe ser un UUID valido")
		return
	}

	msg, err := h.usecase.SendMessage(context.Background(), senderID, roomID, msgReq.Body)
	if err != nil {
		slog.Error("ws: send message fallo", "user_id", senderID, "error", err)
		writeError(conn, "SEND_FAILED", err.Error())
		return
	}

	ack, _ := dto.NewEnvelope(dto.EventMessageSent, dto.MessageSentPayload{
		MessageID: msg.MessageID.String(),
		RoomID:    msg.RoomID.String(),
		SenderID:  msg.SenderID.String(),
		Body:      msg.ContentBody,
		CreatedAt: msg.CreatedAt.Format(time.RFC3339Nano),
	})
	conn.WriteJSON(ack)
}

func (h *Handler) handleTyping(senderID gocql.UUID, payload json.RawMessage) {
	var typing dto.TypingPayload
	if err := json.Unmarshal(payload, &typing); err != nil {
		return
	}

	roomID, err := gocql.ParseUUID(typing.RoomID)
	if err != nil {
		return
	}

	members, err := h.usecase.GetRoomMembers(context.Background(), senderID, roomID)
	if err != nil {
		return
	}

	event, _ := dto.NewEnvelope(dto.EventTyping, dto.TypingPayload{
		RoomID: roomID.String(),
		UserID: senderID.String(),
	})

	data, _ := json.Marshal(event)
	h.hub.BroadcastToRoom(members, data, senderID)
}

func writeError(conn *websocket.Conn, code, message string) {
	env := dto.ErrorEnvelope(code, message)
	conn.WriteJSON(env)
}

func getUserIDFromContext(c *echo.Context) (gocql.UUID, error) {
	val := c.Get("user_id")
	if val == nil {
		return gocql.UUID{}, echo.ErrUnauthorized
	}

	switch v := val.(type) {
	case gocql.UUID:
		return v, nil
	case string:
		return gocql.ParseUUID(v)
	default:
		return gocql.UUID{}, echo.ErrUnauthorized
	}
}
