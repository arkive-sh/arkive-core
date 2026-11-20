//go:build integration

package auth_test

import (
	. "arkive/core/services/auth"
	"encoding/base64"
	"errors"
	"testing"

	"arkive/core/internal/arktest"
)

func TestOpaqueLoginFinalizationCannotBeAppliedToAnotherAttempt(t *testing.T) {
	f := newAuthLifecycleFixture(t)
	f.createOpaqueUser(t, "attemptuser", "attemptuser@example.test", "correct password")

	state, request, err := arktest.OpaqueLoginStart(f.ctx, "correct password")
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.service.StartOpaqueLogin(f.ctx, "attemptuser@example.test", request)
	if err != nil {
		t.Fatal(err)
	}
	_, secondRequest, err := arktest.OpaqueLoginStart(f.ctx, "correct password")
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.service.StartOpaqueLogin(f.ctx, "attemptuser@example.test", secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	response, err := base64.RawURLEncoding.DecodeString(first.LoginResponse)
	if err != nil {
		t.Fatal(err)
	}
	finalization, err := arktest.OpaqueLoginFinish(f.ctx, "correct password", state, response)
	if err != nil {
		t.Fatal(err)
	}
	wrongAttemptFinalization := append([]byte(nil), finalization...)
	if _, err := f.service.FinishOpaqueLogin(f.ctx, second.AttemptID, wrongAttemptFinalization, "192.0.2.5", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("cross-attempt finalization error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := f.service.FinishOpaqueLogin(f.ctx, first.AttemptID, finalization, "192.0.2.5", ""); err != nil {
		t.Fatalf("correct attempt could not finish after cross-attempt rejection: %v", err)
	}
}
