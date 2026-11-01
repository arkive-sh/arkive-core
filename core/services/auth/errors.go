package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidInput       = errors.New("invalid input")
	ErrSessionNotFound    = errors.New("session not found")
	ErrVaultNotConfigured = errors.New("vault is not configured")
	ErrPasswordResetToken = errors.New("password reset token is invalid or expired")

	ErrInvalidProtocolMessage = errors.New("invalid OPAQUE message")
)
