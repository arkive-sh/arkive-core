//go:build integration

package auth_test

import (
	. "arkive/core/services/auth"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"arkive/core/internal/arktest"
	"arkive/core/opaque"
	authrepo "arkive/core/repositories/auth"
	sessionrepo "arkive/core/repositories/session"
	totprepo "arkive/core/repositories/totp"
	usersrepo "arkive/core/repositories/users"
	totpservice "arkive/core/services/totp"
)

type authLifecycleFixture struct {
	env     *arktest.Environment
	ctx     context.Context
	service *Service
	totp    *totpservice.Service
	setup   string
}

func newAuthLifecycleFixture(t *testing.T) *authLifecycleFixture {
	return newAuthLifecycleFixtureWithResetTTL(t, time.Hour)
}

func newAuthLifecycleFixtureWithResetTTL(t *testing.T, resetTTL time.Duration) *authLifecycleFixture {
	t.Helper()
	env := arktest.NewEnvironment(t)
	arktest.InstallOpaqueLibrary(t)
	setupBuffer, err := opaque.GenerateSetup()
	if err != nil {
		t.Fatal(err)
	}
	setup := string(setupBuffer.Bytes())
	setupBuffer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	totp := totpservice.NewService(env.DB, totprepo.New(), setup)
	service := NewService(env.DB, authrepo.New(), sessionrepo.New(), usersrepo.New(), totp, Config{
		SessionTTL:            time.Hour,
		PasswordResetTokenTTL: resetTTL,
		BaseURL:               "https://arkive.example",
		OpaqueServerSetup:     setup,
	})
	return &authLifecycleFixture{env: env, ctx: ctx, service: service, totp: totp, setup: setup}
}

func (f *authLifecycleFixture) createSession(t *testing.T, userID string, expiresAt time.Time) string {
	t.Helper()
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		t.Fatal(err)
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	hash := sha256.Sum256(rawToken)
	if err := sessionrepo.New().CreateSession(f.ctx, f.env.DB, userID, hash[:], expiresAt); err != nil {
		t.Fatal(err)
	}
	return token
}

func sessionTokenHash(token string) []byte {
	rawToken, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(rawToken) != 32 {
		return nil
	}
	hash := sha256.Sum256(rawToken)
	return hash[:]
}

func (f *authLifecycleFixture) createOpaqueUser(t *testing.T, username, email, password string) string {
	t.Helper()
	upload, err := arktest.OpaqueRegistrationUpload(f.ctx, f.setup, username, password)
	if err != nil {
		t.Fatal(err)
	}
	registrationRecord, err := opaque.FinishRegistration(upload)
	if err != nil {
		t.Fatal(err)
	}
	defer registrationRecord.Close()
	user, err := arktest.CreateVerifiedUser(f.ctx, f.env.DB, "OPAQUE Test User", username, email,
		registrationRecord.Bytes(), bytes.Repeat([]byte{4}, 16), []byte("encrypted-master-key"))
	if err != nil {
		t.Fatal(err)
	}
	return user.ID
}

func (f *authLifecycleFixture) finishOpaqueLogin(t *testing.T, email, password string, trustedDeviceToken ...string) LoginUnlockResult {
	t.Helper()
	state, request, err := arktest.OpaqueLoginStart(f.ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	start, err := f.service.StartOpaqueLogin(f.ctx, email, request)
	if err != nil {
		t.Fatal(err)
	}
	response, err := base64.RawURLEncoding.DecodeString(start.LoginResponse)
	if err != nil {
		t.Fatal(err)
	}
	finalization, err := arktest.OpaqueLoginFinish(f.ctx, password, state, response)
	if err != nil {
		t.Fatal(err)
	}
	deviceToken := ""
	if len(trustedDeviceToken) > 0 {
		deviceToken = trustedDeviceToken[0]
	}
	result, err := f.service.FinishOpaqueLogin(f.ctx, start.AttemptID, finalization, "192.0.2.5", deviceToken)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (f *authLifecycleFixture) insertUser(t *testing.T, username, email string) string {
	t.Helper()
	user, err := arktest.CreateVerifiedUser(f.ctx, f.env.DB, "Auth Test User", username, email, []byte("opaque-registration-record"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return user.ID
}

func TestSessionLifecycleValidatesRevokesAndExpiresPersistedSessions(t *testing.T) {
	f := newAuthLifecycleFixture(t)
	userID := f.insertUser(t, "authsession", "authsession@example.test")

	firstToken := f.createSession(t, userID, time.Now().Add(time.Hour))
	secondToken := f.createSession(t, userID, time.Now().Add(time.Hour))
	if got, err := f.service.ValidateSession(f.ctx, firstToken); err != nil || got != userID {
		t.Fatalf("validate first session = %q, error %v; want user %s", got, err, userID)
	}
	if _, _, err := sessionrepo.New().GetSessionByTokenHash(f.ctx, f.env.DB, []byte(firstToken)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("raw bearer token lookup error = %v, want no stored row", err)
	}

	if err := f.service.LogoutSession(f.ctx, firstToken); err != nil {
		t.Fatalf("logout first session: %v", err)
	}
	if _, err := f.service.ValidateSession(f.ctx, firstToken); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("validate logged-out session = %v, want ErrSessionNotFound", err)
	}
	if got, err := f.service.ValidateSession(f.ctx, secondToken); err != nil || got != userID {
		t.Fatalf("logout revoked a different session: user=%q error=%v", got, err)
	}
	if err := f.service.LogoutSession(f.ctx, firstToken); err != nil {
		t.Fatalf("repeated logout: %v", err)
	}

	expiredToken := f.createSession(t, userID, time.Now().Add(-time.Second))
	if _, err := f.service.ValidateSession(f.ctx, expiredToken); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("validate expired session = %v, want ErrSessionNotFound", err)
	}
}

func TestOpaqueLoginWithValidPasswordCreatesPersistedSession(t *testing.T) {
	f := newAuthLifecycleFixture(t)
	userID := f.createOpaqueUser(t, "opaqueuser", "opaqueuser@example.test", "correct horse battery staple")

	login := f.finishOpaqueLogin(t, "opaqueuser@example.test", "correct horse battery staple")
	if login.UserID != userID || !bytes.Equal(login.VaultSalt, bytes.Repeat([]byte{4}, 16)) || string(login.EncryptedMasterKey) != "encrypted-master-key" {
		t.Fatalf("successful login returned unexpected vault credentials: %+v", login)
	}
	if got, err := f.service.ValidateSession(f.ctx, login.SessionToken); err != nil || got != userID {
		t.Fatalf("session created by OPAQUE login = (%q, %v), want user %s", got, err, userID)
	}
	if _, _, err := sessionrepo.New().GetSessionByTokenHash(f.ctx, f.env.DB, sessionTokenHash(login.SessionToken)); err != nil {
		t.Fatalf("session repository cannot find login token hash: %v", err)
	}
	if _, _, err := sessionrepo.New().GetSessionByTokenHash(f.ctx, f.env.DB, []byte(login.SessionToken)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("raw OPAQUE session token lookup error = %v, want no stored row", err)
	}
}

func TestPasswordRecoveryReplacesOpaqueCredentialsConsumesTokenAndRevokesSessions(t *testing.T) {
	f := newAuthLifecycleFixture(t)
	const email = "password-recovery@example.test"
	userID := f.createOpaqueUser(t, "recoveryuser", email, "old password")
	proofPublicKey, proofPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.UpdateRecoveryKey(f.ctx, userID, []byte("encrypted-recovery-key"), proofPublicKey); err != nil {
		t.Fatal(err)
	}
	oldSession := f.createSession(t, userID, time.Now().Add(time.Hour))
	secondOldSession := f.createSession(t, userID, time.Now().Add(time.Hour))

	reset, err := f.service.RequestPasswordReset(f.ctx, email)
	if err != nil || reset.ResetToken == "" {
		t.Fatalf("request password recovery = %+v, error %v", reset, err)
	}
	const newPassword = "new password after recovery"
	registrationStart, err := arktest.OpaqueClient(f.ctx, "registration-start", newPassword, "", "")
	if err != nil {
		t.Fatal(err)
	}
	registrationRequest, err := base64.RawURLEncoding.DecodeString(registrationStart.Request)
	if err != nil {
		t.Fatal(err)
	}
	registrationResponse, err := f.service.StartPasswordResetRegistration(f.ctx, reset.ResetToken, registrationRequest)
	if err != nil {
		t.Fatal(err)
	}
	registrationFinish, err := arktest.OpaqueClient(f.ctx, "registration-finish", newPassword, registrationStart.State,
		base64.RawURLEncoding.EncodeToString(registrationResponse.Bytes()))
	registrationResponse.Close()
	if err != nil {
		t.Fatal(err)
	}
	newRegistrationUpload, err := base64.RawURLEncoding.DecodeString(registrationFinish.RegistrationRecord)
	if err != nil {
		t.Fatal(err)
	}
	newVaultSalt := bytes.Repeat([]byte{8}, 16)
	newMasterKey := []byte("new-encrypted-master-key")
	proofMessage := []byte("arkive:recovery-proof:v1\npassword-reset\n" + userID + "\n" + reset.ResetToken)
	proof := ed25519.Sign(proofPrivateKey, proofMessage)
	validationErrors, err := f.service.CompletePasswordRecovery(f.ctx, reset.ResetToken, newRegistrationUpload,
		newVaultSalt, newMasterKey, proof)
	if err != nil || validationErrors != nil && validationErrors.HasAny() {
		t.Fatalf("complete password recovery: validation=%v error=%v", validationErrors, err)
	}
	if _, err := f.service.LoadPasswordRecoveryVault(f.ctx, reset.ResetToken); !errors.Is(err, ErrPasswordResetToken) {
		t.Fatalf("consumed reset token remains valid: %v", err)
	}
	for _, session := range []string{oldSession, secondOldSession} {
		if _, err := f.service.ValidateSession(f.ctx, session); !errors.Is(err, ErrSessionNotFound) {
			t.Fatalf("existing session survived recovery: %v", err)
		}
	}
	savedUser, err := authrepo.New().GetUserByEmail(f.ctx, f.env.DB, email)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(savedUser.VaultSalt, newVaultSalt) || !bytes.Equal(savedUser.EncryptedMasterKey, newMasterKey) {
		t.Fatal("password recovery did not save the replacement vault credentials")
	}

	newLogin := f.finishOpaqueLogin(t, email, newPassword)
	if newLogin.UserID != userID {
		t.Fatalf("new password authenticated user %q, want %s", newLogin.UserID, userID)
	}
}

func TestPasswordResetHidesUnrecoverableAccountsAndRejectsInvalidProofWithoutMutation(t *testing.T) {
	f := newAuthLifecycleFixture(t)
	f.insertUser(t, "resetuser", "resetuser@example.test")
	vaultSalt := bytes.Repeat([]byte{3}, 16)
	encryptedMasterKey := []byte("encrypted-master-key")
	recoverableUser, err := arktest.CreateVerifiedUser(f.ctx, f.env.DB, "Auth Test User", "recoverable", "recoverable@example.test", []byte("opaque-registration-record"), vaultSalt, encryptedMasterKey)
	if err != nil {
		t.Fatal(err)
	}
	recoverableUserID := recoverableUser.ID

	for _, email := range []string{"unknown@example.test", "resetuser@example.test"} {
		result, err := f.service.RequestPasswordReset(f.ctx, email)
		if err != nil || result.ResetToken != "" || result.ResetURL != "" {
			t.Fatalf("reset request for %q = %+v, error %v; want an empty result", email, result, err)
		}
	}

	proofPublicKey, proofPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encryptedRecoveryKey := []byte("encrypted-recovery-key")
	if err := f.service.UpdateRecoveryKey(f.ctx, recoverableUserID, encryptedRecoveryKey, proofPublicKey); err != nil {
		t.Fatal(err)
	}
	reset, err := f.service.RequestPasswordReset(f.ctx, "recoverable@example.test")
	if err != nil || reset.ResetToken == "" {
		t.Fatalf("request recovery for configured account = %+v, error %v", reset, err)
	}
	resetURL, err := url.Parse(reset.ResetURL)
	if err != nil || resetURL.Query().Get("token") != reset.ResetToken {
		t.Fatalf("reset URL does not carry the returned token: %q, error %v", reset.ResetURL, err)
	}
	vault, err := f.service.LoadPasswordRecoveryVault(f.ctx, reset.ResetToken)
	if err != nil || vault.UserID != recoverableUserID || !bytes.Equal(vault.VaultSalt, vaultSalt) || !bytes.Equal(vault.EncryptedMasterKeyRecovery, encryptedRecoveryKey) {
		t.Fatalf("load recovery vault = %+v, error %v", vault, err)
	}

	sessionToken := f.createSession(t, recoverableUserID, time.Now().Add(time.Hour))
	for name, message := range map[string][]byte{
		"different user":  []byte("arkive:recovery-proof:v1\npassword-reset\nother-user\n" + reset.ResetToken),
		"different token": []byte("arkive:recovery-proof:v1\npassword-reset\n" + recoverableUserID + "\n" + reset.ResetToken + "changed"),
	} {
		t.Run(name, func(t *testing.T) {
			proof := ed25519.Sign(proofPrivateKey, message)
			validationErrors, err := f.service.CompletePasswordRecovery(
				f.ctx,
				reset.ResetToken,
				[]byte("invalid opaque registration"),
				bytes.Repeat([]byte{9}, 16),
				[]byte("new encrypted master key"),
				proof,
			)
			if err != nil || validationErrors == nil || validationErrors["recoveryProof"] == "" {
				t.Fatalf("password recovery with mismatched proof = validations %v, error %v; want proof rejection", validationErrors, err)
			}
			if _, err := f.service.LoadPasswordRecoveryVault(f.ctx, reset.ResetToken); err != nil {
				t.Fatalf("invalid proof consumed the reset token: %v", err)
			}
		})
	}
	if userID, err := f.service.ValidateSession(f.ctx, sessionToken); err != nil || userID != recoverableUserID {
		t.Fatalf("invalid proof revoked the existing session: user=%q error=%v", userID, err)
	}
	storedUser, err := authrepo.New().GetUserByEmail(f.ctx, f.env.DB, "recoverable@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(storedUser.OpaqueRegistrationRecord, []byte("opaque-registration-record")) || !bytes.Equal(storedUser.VaultSalt, vaultSalt) || !bytes.Equal(storedUser.EncryptedMasterKey, encryptedMasterKey) {
		t.Fatal("invalid recovery proof changed the user's password or vault keys")
	}
}

func TestExpiredPasswordRecoveryTokenIsRejected(t *testing.T) {
	f := newAuthLifecycleFixtureWithResetTTL(t, time.Millisecond)
	userID := f.insertUser(t, "expiredreset", "expiredreset@example.test")
	proofPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.UpdateRecoveryKey(f.ctx, userID, []byte("recovery-key"), proofPublicKey); err != nil {
		t.Fatal(err)
	}
	reset, err := f.service.RequestPasswordReset(f.ctx, "expiredreset@example.test")
	if err != nil || reset.ResetToken == "" {
		t.Fatalf("request reset = %+v, error %v", reset, err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := f.service.LoadPasswordRecoveryVault(f.ctx, reset.ResetToken); !errors.Is(err, ErrPasswordResetToken) {
		t.Fatalf("expired recovery token error = %v, want ErrPasswordResetToken", err)
	}
}
