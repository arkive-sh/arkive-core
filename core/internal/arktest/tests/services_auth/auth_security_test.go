package auth_test

import (
	. "arkive/core/services/auth"
	"errors"
	"testing"

	authrepo "arkive/core/repositories/auth"
	sessionrepo "arkive/core/repositories/session"
	usersrepo "arkive/core/repositories/users"
)

func TestValidateSessionRejectsMalformedBearerTokens(t *testing.T) {
	service := NewService(nil, authrepo.New(), sessionrepo.New(), usersrepo.New(), Config{})
	if _, err := service.ValidateSession(t.Context(), "not-a-session-token"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("malformed session token error = %v, want ErrSessionNotFound", err)
	}
}
