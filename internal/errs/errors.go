package errs

import "errors"

var (
	ErrNotMember        = errors.New("el usuario no es miembro de esta sala")
	ErrRoomNotFound     = errors.New("sala no encontrada")
	ErrDirectRoomExists = errors.New("ya existe una sala DIRECT entre estos usuarios")
	ErrInvalidCursor    = errors.New("cursor invalido")
	ErrInvalidRoomType  = errors.New("tipo de sala invalido. Use DIRECT o GROUP")
	ErrInvalidMessage   = errors.New("mensaje invalido")
	ErrRoomTypeMismatch = errors.New("el tipo de sala no coincide con la operacion")
	ErrUnauthorized     = errors.New("no autorizado")
)
