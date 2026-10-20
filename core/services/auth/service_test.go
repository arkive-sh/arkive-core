package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
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
