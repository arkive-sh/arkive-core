package shares

import "errors"

const (
	FileStatusComplete = "complete"

	TokenMinLength = 12
	TokenMaxLength = 128
)

var (
	ErrUnauthorized          = errors.New("unauthorized")
	ErrNotFound              = errors.New("share not found")
	ErrInvalidInput          = errors.New("invalid input")
	ErrShareExists           = errors.New("share already exists")
	ErrAlreadyConsumed       = errors.New("share already consumed")
	ErrSetupIncomplete       = errors.New("folder share is not fully prepared")
	ErrPasswordNotConfigured = errors.New("share password is not configured")
	ErrInvalidCredentials    = errors.New("invalid share password")

	ErrFileIDRequired = errors.New("file id is required")
	ErrTokenInvalid   = errors.New("token must be url-safe and 12-128 chars")
	ErrExpiryInvalid  = errors.New("expiry must be in the future")
)
