package storage

import "errors"

var (
	ErrInvalidID               = errors.New("invalid object ID")
	ErrPermissionDeniedStorage = errors.New("permission denied")
	ErrInvalidSchema           = errors.New("invalid schema")
	ErrTimeout                 = errors.New("storage operation timed out")
)
