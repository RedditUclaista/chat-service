package dto

import "encoding/json"

const (
	EventNewMessage   = "NEW_MESSAGE"
	EventMessageSent  = "MESSAGE_SENT"
	EventMessageRead  = "MESSAGE_READ"
	EventTyping       = "TYPING"
	EventError        = "ERROR"
	EventUserJoined   = "USER_JOINED"
	EventUserLeft     = "USER_LEFT"
)

type Envelope struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
}

type NewMessagePayload struct {
	RoomID string `json:"room_id" validate:"required"`
	Body   string `json:"body" validate:"required"`
}

type MessageSentPayload struct {
	MessageID string `json:"message_id"`
	RoomID    string `json:"room_id"`
	SenderID  string `json:"sender_id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type TypingPayload struct {
	RoomID string `json:"room_id"`
	UserID string `json:"user_id"`
}

type MessageReadPayload struct {
	RoomID    string `json:"room_id"`
	MessageID string `json:"message_id"`
	UserID    string `json:"user_id"`
}

type UserPresencePayload struct {
	RoomID string `json:"room_id"`
	UserID string `json:"user_id"`
}

func NewEnvelope(event string, payload any) (Envelope, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{Event: event, Payload: data}, nil
}

func MustNewEnvelope(event string, payload any) Envelope {
	env, err := NewEnvelope(event, payload)
	if err != nil {
		return Envelope{Event: EventError, Payload: mustMarshal(ErrorPayload{Code: "INTERNAL", Message: "error al crear envelope"})}
	}
	return env
}

func mustMarshal(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func ErrorEnvelope(code, message string) Envelope {
	return Envelope{
		Event:   EventError,
		Payload: mustMarshal(ErrorPayload{Code: code, Message: message}),
	}
}
