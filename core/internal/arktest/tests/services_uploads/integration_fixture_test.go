//go:build integration

package uploads_test

import (
	. "arkive/core/services/uploads"
	"context"
	"encoding/base64"
	"testing"
	"time"

	"arkive/core/internal/arktest"
	"arkive/core/models"
	filerepo "arkive/core/repositories/files"
	folderrepo "arkive/core/repositories/folders"
	settingsrepo "arkive/core/repositories/settings"
	sharerepo "arkive/core/repositories/shares"
	storagerepo "arkive/core/repositories/storage"
	uploadrepo "arkive/core/repositories/uploads"
	usersrepo "arkive/core/repositories/users"
	"arkive/pkg/storage"
)

const uploadTestEncryptedSize int64 = 34
const encryptedChunkEnvelopeOverheadBytes int64 = 29
const testThumbnailMaxEncryptedBytes int64 = 150 * 1024

func reservedUploadSize(encryptedSize int64) int64 {
	if encryptedSize == 0 {
		return 0
	}
	return encryptedSize + testThumbnailMaxEncryptedBytes
}

type uploadIntegrationFixture struct {
	env      *arktest.Environment
	ctx      context.Context
	userID   string
	files    *filerepo.Repository
	users    *usersrepo.Repository
	settings *settingsrepo.Repository
}

func newUploadIntegrationFixture(t *testing.T) *uploadIntegrationFixture {
	t.Helper()
	env := arktest.NewEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	user, err := arktest.CreateVerifiedUser(ctx, env.DB, "Upload User", "uploadtest", "uploadtest@example.test", []byte("registration"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	userID := user.ID
	settings := settingsrepo.New()
	if err := settings.SaveStorageSettings(ctx, env.DB, models.StorageSettings{Provider: "s3"}); err != nil {
		t.Fatal(err)
	}
	return &uploadIntegrationFixture{
		env:      env,
		ctx:      ctx,
		userID:   userID,
		files:    filerepo.New(),
		users:    usersrepo.New(),
		settings: settings,
	}
}

func (f *uploadIntegrationFixture) service(provider storage.Provider) *Service {
	return NewService(
		f.env.DB,
		storagerepo.New(),
		folderrepo.New(),
		f.files,
		f.settings,
		uploadrepo.New(),
		f.users,
		sharerepo.New(),
		provider,
		Config{UploadExpires: time.Hour},
	)
}

func (f *uploadIntegrationFixture) startSinglePart(t *testing.T, service *Service) models.UploadStartResponse {
	t.Helper()
	started, validationErrors, err := service.StartMultipartUploadSession(f.ctx, f.userID, singlePartUploadInput())
	if err != nil {
		t.Fatal(err)
	}
	if validationErrors.HasAny() {
		t.Fatalf("valid upload start failed validation: %v", validationErrors)
	}
	return started
}

func singlePartUploadInput() MultipartUploadStartInput {
	return MultipartUploadStartInput{
		EncryptedSize: uploadTestEncryptedSize, FileChunkSize: 1024, TotalChunks: 1,
		UploadPartSize: 1024, UploadPartCount: 1, EncryptionVersion: 1, SinglePart: true,
	}
}

func (f *uploadIntegrationFixture) completeInput() MultipartUploadCompleteInput {
	return MultipartUploadCompleteInput{
		EncryptedMetadata: base64.StdEncoding.EncodeToString([]byte("metadata-ciphertext")),
		EncryptedFileKey:  base64.StdEncoding.EncodeToString([]byte("file-key-ciphertext")),
		EncryptedManifest: base64.StdEncoding.EncodeToString([]byte("manifest-ciphertext")),
	}
}

func (f *uploadIntegrationFixture) assertUsage(t *testing.T, used, reserved int64) {
	t.Helper()
	actualUsed, actualReserved, err := f.users.GetStorageUsage(f.ctx, f.env.DB, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if actualUsed != used || actualReserved != reserved {
		t.Fatalf("storage usage = used %d, reserved %d; want %d, %d", actualUsed, actualReserved, used, reserved)
	}
}

func (f *uploadIntegrationFixture) assertStatuses(t *testing.T, fileID, sessionID, wantFile, wantSession string) {
	t.Helper()
	file, err := f.files.GetEncryptedFileRecordForUser(f.ctx, f.env.DB, fileID, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if file.UploadStatus != wantFile {
		t.Errorf("file status = %q, want %q", file.UploadStatus, wantFile)
	}
	var sessionStatus string
	if err := f.env.DB.QueryRow(f.ctx, `SELECT status FROM upload_sessions WHERE id = $1`, sessionID).Scan(&sessionStatus); err != nil {
		t.Fatal(err)
	}
	if sessionStatus != wantSession {
		t.Errorf("upload session status = %q, want %q", sessionStatus, wantSession)
	}
}
