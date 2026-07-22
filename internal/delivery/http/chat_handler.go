package http

import (
	"errors"
	"net/http"

	"github.com/RedditUclaista/chat-service/internal/dto"
	"github.com/RedditUclaista/chat-service/internal/errs"
	"github.com/RedditUclaista/chat-service/internal/usecases"
	ws "github.com/RedditUclaista/chat-service/internal/delivery/websocket"
	"github.com/gocql/gocql"
	"github.com/labstack/echo/v5"
)

type ChatHandler struct {
	usecase   *usecases.ChatUseCase
	wsHandler *ws.Handler
}

func NewChatHandler(uc *usecases.ChatUseCase, ws *ws.Handler) *ChatHandler {
	return &ChatHandler{usecase: uc, wsHandler: ws}
}

func (h *ChatHandler) RegisterRoutes(g *echo.Group) {
	g.POST("/rooms", h.CreateRoom)
	g.GET("/rooms", h.GetUserRooms)
	g.GET("/rooms/:id/messages", h.GetRoomMessages)
}

func (h *ChatHandler) RegisterWSRoute(g *echo.Group) {
	g.GET("/ws", h.wsHandler.HandleWS)
}

func (h *ChatHandler) CreateRoom(c *echo.Context) error {
	creatorID, err := getUserIDFromContext(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "usuario no autenticado")
	}

	var req dto.CreateRoomRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "body invalido: "+err.Error())
	}
	if err := c.Validate(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	room, err := h.usecase.CreateRoom(c.Request().Context(), creatorID, req)
	if err != nil {
		if errors.Is(err, errs.ErrDirectRoomExists) || errors.Is(err, errs.ErrInvalidRoomType) {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, room)
}

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

func (h *ChatHandler) GetRoomMessages(c *echo.Context) error {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "usuario no autenticado")
	}

	roomID, err := gocql.ParseUUID(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "room_id invalido: debe ser un UUID")
	}

	var req dto.GetMessagesRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "parametros invalidos")
	}

	result, err := h.usecase.GetRoomMessages(c.Request().Context(), userID, roomID, req)
	if err != nil {
		if errors.Is(err, errs.ErrNotMember) {
			return echo.NewHTTPError(http.StatusForbidden, err.Error())
		}
		if errors.Is(err, errs.ErrInvalidCursor) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, result)
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
