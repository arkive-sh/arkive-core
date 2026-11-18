//go:build integration

package shares_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"arkive/core/database"
	"arkive/core/internal/arktest"
	"arkive/core/models"
	"arkive/core/opaque"
	filerepo "arkive/core/repositories/files"
	folderrepo "arkive/core/repositories/folders"
	sharerepo "arkive/core/repositories/shares"
	sharesvc "arkive/core/services/shares"
)

func TestOpaqueSharePasswordProtectsCreationAndLoginAttempts(t *testing.T) {
	env := arktest.NewEnvironment(t)
	arktest.InstallOpaqueLibrary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	setupBuffer, err := opaque.GenerateSetup()
	if err != nil {
		t.Fatal(err)
	}
	serverSetup := string(setupBuffer.Bytes())
	setupBuffer.Close()

	owner, err := arktest.CreateVerifiedUser(ctx, env.DB, "Share Password User", "sharepassword", "sharepassword@example.test", []byte("registration"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	files := filerepo.New()
	service := sharesvc.NewService(env.DB, files, folderrepo.New(), sharerepo.New(), serverSetup)
	firstFile := createPasswordTestFile(t, ctx, files, env.DB, owner.ID, "first encrypted file")
	secondFile := createPasswordTestFile(t, ctx, files, env.DB, owner.ID, "second encrypted file")

	firstToken := strings.Repeat("a", sharesvc.TokenMinLength)
	secondToken := strings.Repeat("b", sharesvc.TokenMinLength)
	firstPassword := "First Share Password!"
	secondPassword := "Second Share Password!"
	firstUpload := createSharePasswordUpload(t, ctx, service, firstToken, firstPassword, owner.ID, "")
	firstShare, validationErrors, err := service.CreateShare(ctx, sharesvc.CreateInput{
		FileID: firstFile.ID, OwnerUserID: owner.ID, Token: firstToken, RequirePassword: true,
		OpaqueRegistrationUpload: firstUpload, AllowPreview: true,
		EncryptedShareKey:        base64.StdEncoding.EncodeToString([]byte("first share key")),
		EncryptedFileKeyForShare: base64.StdEncoding.EncodeToString([]byte("first wrapped file key")),
	})
	if err != nil || validationErrors.HasAny() {
		t.Fatalf("create password protected share: validation=%v error=%v", validationErrors, err)
	}
	if firstShare.Status != sharesvc.ShareStatusActive || len(firstShare.OpaqueRegistrationRecord) == 0 {
		t.Fatalf("created password share = %+v, want active with OPAQUE registration", firstShare)
	}

	secondShare, validationErrors, err := service.CreateShare(ctx, sharesvc.CreateInput{
		FileID: secondFile.ID, OwnerUserID: owner.ID, Token: secondToken, AllowPreview: true,
		EncryptedShareKey:        base64.StdEncoding.EncodeToString([]byte("second share key")),
		EncryptedFileKeyForShare: base64.StdEncoding.EncodeToString([]byte("second wrapped file key")),
	})
	if err != nil || validationErrors.HasAny() {
		t.Fatalf("create unprotected share: validation=%v error=%v", validationErrors, err)
	}
	secondUpload := createSharePasswordUpload(t, ctx, service, secondToken, secondPassword, owner.ID, secondShare.ID)
	secondShare, err = service.FinishPasswordRegistration(ctx, secondShare.ID, owner.ID, secondUpload)
	if err != nil || len(secondShare.OpaqueRegistrationRecord) == 0 {
		t.Fatalf("set password on existing share: share=%+v error=%v", secondShare, err)
	}
	staleClientState, staleRequest, err := arktest.OpaqueLoginStart(ctx, secondPassword)
	if err != nil {
		t.Fatal(err)
	}
	staleStart, err := service.StartPasswordLogin(ctx, secondToken, staleRequest)
	if err != nil {
		t.Fatalf("start OPAQUE login before password change: %v", err)
	}
	staleResponse, err := base64.RawURLEncoding.DecodeString(staleStart.LoginResponse)
	if err != nil {
		t.Fatal(err)
	}
	staleFinalization, err := arktest.OpaqueLoginFinish(ctx, secondPassword, staleClientState, staleResponse)
	if err != nil {
		t.Fatal(err)
	}
	newPassword := "Updated Share Password!"
	newUpload := createSharePasswordUpload(t, ctx, service, secondToken, newPassword, owner.ID, secondShare.ID)
	if _, err := service.FinishPasswordRegistration(ctx, secondShare.ID, owner.ID, newUpload); err != nil {
		t.Fatalf("change existing share password: %v", err)
	}
	if _, err := service.FinishPasswordLogin(ctx, secondToken, staleStart.AttemptID, staleFinalization); !errors.Is(err, sharesvc.ErrInvalidCredentials) {
		t.Fatalf("finish login started before password change = %v, want invalid credentials", err)
	}
	_, invalidRequest, err := arktest.OpaqueLoginStart(ctx, firstPassword)
	if err != nil {
		t.Fatal(err)
	}
	invalidStart, err := service.StartPasswordLogin(ctx, firstToken, invalidRequest)
	if err != nil {
		t.Fatalf("start OPAQUE login for malformed finalization: %v", err)
	}
	if _, err := service.FinishPasswordLogin(ctx, firstToken, invalidStart.AttemptID, []byte("invalid finalization")); !errors.Is(err, sharesvc.ErrInvalidCredentials) {
		t.Fatalf("malformed OPAQUE finalization = %v, want invalid credentials", err)
	}
	clientState, loginRequest, err := arktest.OpaqueLoginStart(ctx, firstPassword)
	if err != nil {
		t.Fatal(err)
	}
	loginStart, err := service.StartPasswordLogin(ctx, firstToken, loginRequest)
	if err != nil {
		t.Fatalf("start OPAQUE share login: %v", err)
	}
	loginResponse, err := base64.RawURLEncoding.DecodeString(loginStart.LoginResponse)
	if err != nil {
		t.Fatal(err)
	}
	finalization, err := arktest.OpaqueLoginFinish(ctx, firstPassword, clientState, loginResponse)
	if err != nil {
		t.Fatal(err)
	}
	replay := bytes.Clone(finalization)
	if _, err := service.FinishPasswordLogin(ctx, secondToken, loginStart.AttemptID, bytes.Clone(finalization)); !errors.Is(err, sharesvc.ErrInvalidCredentials) {
		t.Fatalf("finish share A login against share B = %v, want invalid credentials", err)
	}
	if _, err := service.FinishPasswordLogin(ctx, firstToken, loginStart.AttemptID, finalization); err != nil {
		t.Fatalf("finish valid OPAQUE share login: %v", err)
	}
	if _, err := service.FinishPasswordLogin(ctx, firstToken, loginStart.AttemptID, replay); !errors.Is(err, sharesvc.ErrInvalidCredentials) {
		t.Fatalf("replay consumed share login attempt = %v, want invalid credentials", err)
	}

	if _, err := service.RemovePassword(ctx, secondShare.ID, owner.ID); err != nil {
		t.Fatalf("remove share password: %v", err)
	}
	if _, err := service.StartPasswordLogin(ctx, secondToken, loginRequest); !errors.Is(err, sharesvc.ErrPasswordNotConfigured) {
		t.Fatalf("start login after password removal = %v, want ErrPasswordNotConfigured", err)
	}
}

func createPasswordTestFile(t *testing.T, ctx context.Context, files *filerepo.Repository, db database.PgExecutor, ownerID, marker string) models.File {
	t.Helper()
	file, err := files.CreateEncryptedFile(ctx, db, models.File{
		UserID: ownerID, EncryptedMetadata: []byte(marker + " metadata"), EncryptedFileKey: []byte(marker + " file key"),
		EncryptedManifest: []byte(marker + " manifest"), EncryptionVersion: 1, ChunkSize: 1024,
		ChunkCount: 1, ActualEncryptedSize: 42, UploadStatus: "complete",
	})
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func createSharePasswordUpload(t *testing.T, ctx context.Context, service *sharesvc.Service, identifier, password, ownerID, shareID string) []byte {
	t.Helper()
	start, err := arktest.OpaqueClient(ctx, "registration-start", password, "", "")
	if err != nil {
		t.Fatal(err)
	}
	request, err := base64.RawURLEncoding.DecodeString(start.Request)
	if err != nil {
		t.Fatal(err)
	}
	var response string
	if shareID == "" {
		response, err = service.StartNewSharePasswordRegistration(identifier, request)
	} else {
		response, err = service.StartPasswordRegistration(ctx, shareID, ownerID, request)
	}
	if err != nil {
		t.Fatalf("start share password registration: %v", err)
	}
	finish, err := arktest.OpaqueClient(ctx, "registration-finish", password, start.State, response)
	if err != nil {
		t.Fatal(err)
	}
	upload, err := base64.RawURLEncoding.DecodeString(finish.RegistrationRecord)
	if err != nil {
		t.Fatal(err)
	}
	return upload
}
