//go:build integration

package uploads_test

import (
	"testing"

	"arkive/core/models"
	"arkive/pkg/storage"
)

func TestCompletingUploadCountsEncryptedThumbnailAgainstStorageQuota(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	service := f.service(f.env.Storage)
	started := f.startSinglePart(t, service)

	const thumbnailSize int64 = 73
	if err := f.settings.SaveStorageSettings(f.ctx, f.env.DB, models.StorageSettings{
		Provider: "s3", MaxStorageBytes: uploadTestEncryptedSize + thumbnailSize,
	}); err != nil {
		t.Fatal(err)
	}
	objectKey, err := storage.BuildObjectKey(f.userID, started.FileID)
	if err != nil {
		t.Fatal(err)
	}
	thumbnailKey, err := storage.BuildThumbnailObjectKey(f.userID, started.FileID)
	if err != nil {
		t.Fatal(err)
	}
	f.env.Storage.PutObject(objectKey, make([]byte, uploadTestEncryptedSize))
	f.env.Storage.PutObject(thumbnailKey, make([]byte, thumbnailSize))

	input := f.completeInput()
	input.HasThumbnail = true
	input.ThumbnailSize = thumbnailSize
	if err := service.CompleteMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID, input); err != nil {
		t.Fatalf("complete upload with thumbnail at exact storage limit: %v", err)
	}
	file, err := f.files.GetEncryptedFileRecordForUser(f.ctx, f.env.DB, started.FileID, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if file.ThumbnailSizeBytes != thumbnailSize {
		t.Fatalf("persisted encrypted thumbnail size = %d, want %d", file.ThumbnailSizeBytes, thumbnailSize)
	}
	f.assertUsage(t, uploadTestEncryptedSize+thumbnailSize, 0)
	if _, ok := f.env.Storage.GetObject(thumbnailKey); !ok {
		t.Fatal("completed thumbnail object is missing")
	}
}
