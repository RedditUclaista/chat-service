package dto

import (
	"github.com/gocql/gocql"
)

const DefaultMessageLimit = 30

type CreateRoomRequest struct {
	RoomType  string       `json:"room_type" validate:"required,oneof=DIRECT GROUP"`
	Name      string       `json:"name,omitempty"`
	MemberIDs []gocql.UUID `json:"member_ids" validate:"required,min=1"`
}

type GetMessagesRequest struct {
	Cursor string `query:"cursor"`
	Limit  int    `query:"limit"`
}

func (r *GetMessagesRequest) GetLimit() int {
	if r.Limit <= 0 || r.Limit > 100 {
		return DefaultMessageLimit
	}
	return r.Limit
}
