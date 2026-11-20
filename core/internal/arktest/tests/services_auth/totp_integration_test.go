//go:build integration

package auth_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	totpservice "arkive/core/services/totp"
)

func TestTOTPEnrollmentRequiresVerificationAndLoginRejectsReusedProofs(t *testing.T) {
	f := newAuthLifecycleFixture(t)
	userID := f.createOpaqueUser(t, "totpenrollment", "totpenrollment@example.test", "test-password")

	setup, err := f.totp.BeginEnrollment(f.ctx, userID, "totpenrollment")
	if err != nil {
		t.Fatalf("begin TOTP enrollment: %v", err)
	}
	enrollmentCode, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate enrollment code: %v", err)
	}
	recoveryCodes, err := f.totp.ConfirmEnrollment(f.ctx, userID, enrollmentCode)
	if err != nil {
		t.Fatalf("confirm TOTP enrollment: %v", err)
	}
	if len(recoveryCodes) != 10 {
		t.Fatalf("generated %d recovery codes, want 10", len(recoveryCodes))
	}

	login := f.finishOpaqueLogin(t, "totpenrollment@example.test", "test-password")
	if !login.MFARequired || login.SessionToken != "" {
		t.Fatalf("first login result = MFA %t, session %q; want MFA challenge without a session", login.MFARequired, login.SessionToken)
	}
	// The next time-step remains inside the verifier's one-step clock-skew window.
	loginCode, err := totp.GenerateCode(setup.Secret, time.Now().Add(30*time.Second))
	if err != nil {
		t.Fatalf("generate login code: %v", err)
	}
	verified, err := f.service.CompleteTOTPLogin(f.ctx, login.MFAChallenge, loginCode, "", true, "192.0.2.8")
	if err != nil {
		t.Fatalf("complete TOTP login: %v", err)
	}
	if verified.SessionToken == "" || len(verified.RecoveryCodes) != 0 {
		t.Fatalf("verified login returned session=%t and %d recovery codes; want a session and no new codes", verified.SessionToken != "", len(verified.RecoveryCodes))
	}
	remaining := time.Until(verified.TrustedDeviceExpiry)
	if verified.TrustedDeviceToken == "" || remaining < totpservice.TrustedDeviceTTL-time.Second || remaining > totpservice.TrustedDeviceTTL {
		t.Fatalf("remembered login expiry has %s remaining, want 7 days", remaining)
	}
	rememberedLogin := f.finishOpaqueLogin(t, "totpenrollment@example.test", "test-password", verified.TrustedDeviceToken)
	if rememberedLogin.MFARequired || rememberedLogin.SessionToken == "" {
		t.Fatal("remembered device login still required MFA or failed to create a session")
	}
	unknownDeviceLogin := f.finishOpaqueLogin(t, "totpenrollment@example.test", "test-password", strings.Repeat("A", 43))
	if !unknownDeviceLogin.MFARequired {
		t.Fatal("unrecognized trusted-device token bypassed MFA")
	}

	replayChallenge := f.finishOpaqueLogin(t, "totpenrollment@example.test", "test-password")
	if !replayChallenge.MFARequired {
		t.Fatal("repeat OPAQUE login did not require MFA")
	}
	if _, err := f.service.CompleteTOTPLogin(f.ctx, replayChallenge.MFAChallenge, loginCode, "", false, "192.0.2.8"); !errors.Is(err, totpservice.ErrInvalidCode) {
		t.Fatalf("reused TOTP code error = %v, want ErrInvalidCode", err)
	}

	if _, err := f.service.CompleteTOTPLogin(f.ctx, replayChallenge.MFAChallenge, "", recoveryCodes[0], false, "192.0.2.8"); err != nil {
		t.Fatalf("complete login with recovery code: %v", err)
	}
	secondRecoveryChallenge := f.finishOpaqueLogin(t, "totpenrollment@example.test", "test-password")
	if _, err := f.service.CompleteTOTPLogin(f.ctx, secondRecoveryChallenge.MFAChallenge, "", recoveryCodes[0], false, "192.0.2.8"); !errors.Is(err, totpservice.ErrInvalidCode) {
		t.Fatalf("reused recovery code error = %v, want ErrInvalidCode", err)
	}

	attemptLimitChallenge := f.finishOpaqueLogin(t, "totpenrollment@example.test", "test-password")
	for attempt := 0; attempt < 5; attempt++ {
		if _, err := f.service.CompleteTOTPLogin(f.ctx, attemptLimitChallenge.MFAChallenge, "bad", "", false, "192.0.2.8"); !errors.Is(err, totpservice.ErrInvalidCode) {
			t.Fatalf("invalid factor attempt %d error = %v, want ErrInvalidCode", attempt+1, err)
		}
	}
	if _, err := f.service.CompleteTOTPLogin(f.ctx, attemptLimitChallenge.MFAChallenge, "bad", "", false, "192.0.2.8"); !errors.Is(err, totpservice.ErrChallengeExpired) {
		t.Fatalf("expired MFA challenge error = %v, want ErrChallengeExpired", err)
	}
}
