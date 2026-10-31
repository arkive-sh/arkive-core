package folders

import "errors"

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrNotFound      = errors.New("not found")
	ErrEntryNotFound = errors.New("entry not found")
	ErrInvalidMove   = errors.New("invalid move")
)
