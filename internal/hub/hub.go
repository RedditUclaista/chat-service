package hub

import (
	"log/slog"
	"sync"

	"github.com/gocql/gocql"
	"github.com/gorilla/websocket"
)

type Hub struct {
	mu       sync.RWMutex
	rooms    map[gocql.UUID]map[*websocket.Conn]struct{}
	userRoom map[*websocket.Conn]gocql.UUID
}

func New() *Hub {
	return &Hub{
		rooms:    make(map[gocql.UUID]map[*websocket.Conn]struct{}),
		userRoom: make(map[*websocket.Conn]gocql.UUID),
	}
}

func (h *Hub) Register(userID gocql.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.rooms[userID] == nil {
		h.rooms[userID] = make(map[*websocket.Conn]struct{})
	}
	h.rooms[userID][conn] = struct{}{}
	h.userRoom[conn] = userID
}

func (h *Hub) Unregister(userID gocql.UUID, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if conns := h.rooms[userID]; conns != nil {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.rooms, userID)
		}
	}
	delete(h.userRoom, conn)
}

func (h *Hub) SendToUser(userID gocql.UUID, data []byte) {
	h.mu.RLock()
	conns := h.rooms[userID]
	h.mu.RUnlock()

	for conn := range conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			slog.Warn("hub: error enviando a conexion", "user_id", userID, "error", err)
			go h.Unregister(userID, conn)
			conn.Close()
		}
	}
}

func (h *Hub) SendToRoom(roomMembers []gocql.UUID, data []byte) {
	for _, uid := range roomMembers {
		h.SendToUser(uid, data)
	}
}

func (h *Hub) BroadcastToRoom(memberIDs []gocql.UUID, data []byte, excludeSender gocql.UUID) {
	for _, uid := range memberIDs {
		if uid == excludeSender {
			continue
		}
		h.SendToUser(uid, data)
	}
}

func (h *Hub) IsConnected(userID gocql.UUID) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms[userID]) > 0
}
