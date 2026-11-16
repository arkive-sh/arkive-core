//go:build integration

package setup_test

import (
	. "arkive/core/services/setup"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"arkive/core/internal/arktest"
	"arkive/core/models"
	"arkive/core/opaque"
	authrepo "arkive/core/repositories/auth"
	settingsrepo "arkive/core/repositories/settings"
	usersrepo "arkive/core/repositories/users"
)

type setupLifecycleFixture struct {
	env     *arktest.Environment
	ctx     context.Context
	service *Service
	setup   string
}

func newSetupLifecycleFixture(t *testing.T) *setupLifecycleFixture {
	t.Helper()
	env := arktest.NewEnvironment(t)
	arktest.InstallOpaqueLibrary(t)
	setupBuffer, err := opaque.GenerateSetup()
	if err != nil {
		t.Fatal(err)
	}
	serverSetup := string(setupBuffer.Bytes())
	setupBuffer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	service := NewService(env.DB, authrepo.New(), usersrepo.New(), settingsrepo.New(), serverSetup)
	return &setupLifecycleFixture{env: env, ctx: ctx, service: service, setup: serverSetup}
}

func (f *setupLifecycleFixture) registrationUpload(t *testing.T, username, password string) []byte {
	t.Helper()
	start, err := arktest.OpaqueClient(f.ctx, "registration-start", password, "", "")
	if err != nil {
		t.Fatal(err)
	}
	request, err := base64.RawURLEncoding.DecodeString(start.Request)
	if err != nil {
		t.Fatal(err)
	}
	response, err := f.service.StartOpaqueRegistration(f.ctx, username, request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := arktest.OpaqueClient(f.ctx, "registration-finish", password, start.State,
		base64.RawURLEncoding.EncodeToString(response.Bytes()))
	response.Close()
	if err != nil {
		t.Fatal(err)
	}
	upload, err := base64.RawURLEncoding.DecodeString(result.RegistrationRecord)
	if err != nil {
		t.Fatal(err)
	}
	return upload
}

func (f *setupLifecycleFixture) initialAdminInput(upload []byte) InitialAdminInput {
	return InitialAdminInput{
		Name:                     "Setup Test User",
		Username:                 "setupuser",
		Email:                    "setupuser@example.test",
		OpaqueRegistrationRecord: upload,
		VaultSalt:                make([]byte, 16),
		EncryptedMasterKey:       []byte("encrypted-master-key"),
		Storage: models.StorageSettings{
			Provider:          "s3",
			S3AccessKeyID:     "test-access-key",
			S3SecretAccessKey: "test-secret-key",
			S3Bucket:          "test-bucket",
			S3Endpoint:        "https://storage.example.test",
			S3Region:          "auto",
		},
	}
}

func TestInitialSetupCanBeRetriedAfterAnInterruptedRegistration(t *testing.T) {
	f := newSetupLifecycleFixture(t)
	firstAttempt, err := arktest.OpaqueClient(f.ctx, "registration-start", "setup password", "", "")
	if err != nil {
		t.Fatal(err)
	}
	firstRequest, err := base64.RawURLEncoding.DecodeString(firstAttempt.Request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.StartOpaqueRegistration(f.ctx, "setupuser", firstRequest); err != nil {
		t.Fatal(err)
	}
	if initialized, err := f.service.IsInitialized(f.ctx); err != nil || initialized {
		t.Fatalf("instance became initialized before registration completed: initialized=%t error=%v", initialized, err)
	}

	upload := f.registrationUpload(t, "setupuser", "setup password")
	user, validationErrors, err := f.service.CreateInitialAdmin(f.ctx, f.initialAdminInput(upload))
	if err != nil || validationErrors != nil && validationErrors.HasAny() {
		t.Fatalf("complete retried setup: validation=%v error=%v", validationErrors, err)
	}
	if user.ID == "" {
		t.Fatal("successful setup returned no user ID")
	}
	if initialized, err := f.service.IsInitialized(f.ctx); err != nil || !initialized {
		t.Fatalf("instance initialization after setup: initialized=%t error=%v", initialized, err)
	}
	duplicateUpload, err := arktest.OpaqueRegistrationUpload(f.ctx, f.setup, "setupuser", "setup password")
	if err != nil {
		t.Fatal(err)
	}
	if _, validationErrors, err := f.service.CreateInitialAdmin(f.ctx, f.initialAdminInput(duplicateUpload)); !errors.Is(err, ErrAlreadyInitialized) || validationErrors != nil {
		t.Fatalf("second setup attempt = validations %v, error %v; want ErrAlreadyInitialized", validationErrors, err)
	}
}

func TestRecoverySetupTokenExpiresAndIsConsumedWhenRecoveryKeyIsSaved(t *testing.T) {
	f := newSetupLifecycleFixture(t)
	user, validationErrors, err := f.service.CreateInitialAdmin(f.ctx, f.initialAdminInput(f.registrationUpload(t, "setupuser", "setup password")))
	if err != nil || validationErrors != nil && validationErrors.HasAny() {
		t.Fatalf("create initial user: validation=%v error=%v", validationErrors, err)
	}
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	token, err := f.service.IssueRecoverySetupToken(f.ctx, user.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if valid, err := f.service.HasValidRecoverySetupToken(f.ctx, token); err != nil || !valid {
		t.Fatalf("new recovery setup token valid=%t error=%v", valid, err)
	}
	if got, err := f.service.GetRecoverySetupUser(f.ctx, token); err != nil || got.ID != user.ID {
		t.Fatalf("recovery setup token user = %+v, error %v", got, err)
	}
	if err := f.service.SaveRecoveryWrappedMasterKey(f.ctx, token, []byte("encrypted-recovery-key"), publicKey); err != nil {
		t.Fatal(err)
	}
	if valid, err := f.service.HasValidRecoverySetupToken(f.ctx, token); err != nil || valid {
		t.Fatalf("consumed recovery setup token valid=%t error=%v", valid, err)
	}
	if err := f.service.SaveRecoveryWrappedMasterKey(f.ctx, token, []byte("replay"), publicKey); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("reusing consumed recovery setup token = %v, want pgx.ErrNoRows", err)
	}

	expiredToken, err := f.service.IssueRecoverySetupToken(f.ctx, user.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.DB.Exec(f.ctx, `UPDATE users SET recovery_setup_token_expires_at = now() - interval '1 second' WHERE id = $1`, user.ID); err != nil {
		t.Fatal(err)
	}
	if valid, err := f.service.HasValidRecoverySetupToken(f.ctx, expiredToken); err != nil || valid {
		t.Fatalf("expired recovery setup token valid=%t error=%v", valid, err)
	}
	if err := f.service.SaveRecoveryWrappedMasterKey(f.ctx, expiredToken, []byte("late-key"), publicKey); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("using expired recovery setup token = %v, want pgx.ErrNoRows", err)
	}
}
