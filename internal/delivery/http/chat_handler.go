package http

import (
	"net/http"

	"github.com/gocql/gocql"
	"github.com/labstack/echo/v5"
	"github.com/RedditUclaista/chat-service/internal/dto"
	"github.com/RedditUclaista/chat-service/internal/usecases"
)

// ChatHandler agrupa los handlers HTTP del chat.
// Solo deserializa el request, llama al usecase y escribe la respuesta.
// Cero lógica de negocio aquí.
type ChatHandler struct {
	usecase *usecases.ChatUseCase
}

func NewChatHandler(uc *usecases.ChatUseCase) *ChatHandler {
	return &ChatHandler{usecase: uc}
}

// RegisterRoutes registra los 3 endpoints REST en el grupo dado.
func (h *ChatHandler) RegisterRoutes(g *echo.Group) {
	g.POST("/rooms", h.CreateRoom)
	g.GET("/rooms", h.GetUserRooms)
	g.GET("/rooms/:id/messages", h.GetRoomMessages)
}

// ─────────────────────────────────────────────────────────────────────────────
// POST /api/v1/chats/rooms
// ─────────────────────────────────────────────────────────────────────────────

func (h *ChatHandler) CreateRoom(c *echo.Context) error {
	creatorID, err := getUserIDFromContext(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "usuario no autenticado")
	}

	var req dto.CreateRoomRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "body inválido: "+err.Error())
	}
	if err := c.Validate(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	room, err := h.usecase.CreateRoom(c.Request().Context(), creatorID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, room)
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /api/v1/chats/rooms
// ─────────────────────────────────────────────────────────────────────────────

func (h *ChatHandler) GetUserRooms(c *echo.Context) error {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "usuario no autenticado")
	}

	rooms, err := h.usecase.GetUserRooms(c.Request().Context(), userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if rooms == nil {
		rooms = []dto.RoomSummaryResponse{}
	}

	return c.JSON(http.StatusOK, rooms)
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /api/v1/chats/rooms/:id/messages
// ─────────────────────────────────────────────────────────────────────────────

func (h *ChatHandler) GetRoomMessages(c *echo.Context) error {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "usuario no autenticado")
	}

	roomID, err := gocql.ParseUUID(c.PathParam("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "room_id inválido: debe ser un UUID")
	}

	var req dto.GetMessagesRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "parámetros inválidos")
	}
	if req.Limit == 0 {
		req.Limit = 30
	}

	result, err := h.usecase.GetRoomMessages(c.Request().Context(), userID, roomID, req)
	if err != nil {
		if err.Error() == "acceso denegado: el usuario no es miembro de esta sala" {
			return echo.NewHTTPError(http.StatusForbidden, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, result)
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper: extraer userID del contexto JWT
// ─────────────────────────────────────────────────────────────────────────────

// getUserIDFromContext extrae el UUID del usuario autenticado.
// El middleware JWT lo inyecta con la key "user_id" antes de llegar al handler.
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
