package auth

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"
)

func TestVerifyPasswordRecoveryProof(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	userID := "user-id"
	resetToken := "one-time-reset-token"
	message := []byte("arkive:recovery-proof:v1\npassword-reset\n" + userID + "\n" + resetToken)
	proof := ed25519.Sign(privateKey, message)

	if !verifyPasswordRecoveryProof(userID, resetToken, publicKey, proof) {
		t.Fatal("valid recovery proof was rejected")
	}
	if verifyPasswordRecoveryProof(userID, resetToken+"changed", publicKey, proof) {
		t.Fatal("proof was accepted for a different reset token")
	}
	if verifyPasswordRecoveryProof(userID+"changed", resetToken, publicKey, proof) {
		t.Fatal("proof was accepted for a different user")
	}
}

func TestOpaqueLoginStateIsEncryptedAndBoundToAttempt(t *testing.T) {
	service := &Service{opaqueSetup: "server-setup-secret"}
	userID := "11111111-1111-1111-1111-111111111111"
	attemptID := "22222222-2222-4222-8222-222222222222"
	expiresAt := time.Unix(1_800_000_000, 0).UTC()
	plaintext := []byte("serialized OPAQUE server state")

	sealed, err := service.sealOpaqueLoginState(attemptID, &userID, expiresAt, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, plaintext) {
		t.Fatal("stored state contains the plaintext OPAQUE state")
	}
	opened, err := service.openOpaqueLoginState(attemptID, &userID, expiresAt, sealed)
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatalf("sealed state did not round-trip: %v", err)
	}

	for name, changed := range map[string]struct {
		attemptID string
		userID    *string
		expiresAt time.Time
	}{
		"attempt": {attemptID: "33333333-3333-4333-8333-333333333333", userID: &userID, expiresAt: expiresAt},
		"user":    {attemptID: attemptID, userID: nil, expiresAt: expiresAt},
		"expiry":  {attemptID: attemptID, userID: &userID, expiresAt: expiresAt.Add(time.Second)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.openOpaqueLoginState(changed.attemptID, changed.userID, changed.expiresAt, sealed); err == nil {
				t.Fatal("state authenticated against modified attempt metadata")
			}
		})
	}
}

func TestSessionTokenHashRejectsInvalidTokens(t *testing.T) {
	rawToken := bytes.Repeat([]byte{0x5a}, 32)
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	expected := sha256.Sum256(rawToken)
	if got := hashSessionToken(token); !bytes.Equal(got, expected[:]) {
		t.Fatal("session token did not produce its expected hash")
	}
	if hashSessionToken("not-a-session-token") != nil {
		t.Fatal("invalid session token was accepted")
	}
}
