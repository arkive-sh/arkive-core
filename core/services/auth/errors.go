package auth

import (
	"errors"
	"time"
)

var (
	ErrInvalidCredentials     = errors.New("invalid credentials")
	ErrInvalidInput           = errors.New("invalid input")
	ErrSessionNotFound        = errors.New("session not found")
	ErrVaultNotConfigured     = errors.New("vault is not configured")
	ErrPasswordResetToken     = errors.New("password reset token is invalid or expired")
	ErrInvalidProtocolMessage = errors.New("invalid OPAQUE message")
	ErrLoginCooldown          = errors.New("too many failed sign-in attempts")
)

// LoginCooldownError reports a password sign-in cooldown and its remaining duration.
type LoginCooldownError struct {
	RetryAfter time.Duration
}

// Error returns the generic sign-in cooldown message.
func (e *LoginCooldownError) Error() string { return ErrLoginCooldown.Error() }

// Unwrap allows callers to match ErrLoginCooldown.
func (e *LoginCooldownError) Unwrap() error { return ErrLoginCooldown }
