//go:build integration

package uploads_test

import (
	. "arkive/core/services/uploads"
	"bytes"
	"errors"
	"strings"
	"testing"

	"arkive/pkg/storage"
)

func TestSinglePartUploadCompletesAndRejectsWrongOwnerAndReplay(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	service := f.service(f.env.Storage)

	const encryptedSize = uploadTestEncryptedSize // one 5-byte encrypted chunk plus its 29-byte envelope
	started := f.startSinglePart(t, service)
	if started.FileID == "" || started.UploadSessionID == "" || !strings.HasPrefix(started.UploadURL, "memory://upload/") {
		t.Fatalf("unexpected upload start response: %+v", started)
	}

	f.assertUsage(t, 0, reservedUploadSize(encryptedSize))

	objectKey, err := storage.BuildObjectKey(f.userID, started.FileID)
	if err != nil {
		t.Fatal(err)
	}
	encryptedObject := make([]byte, encryptedSize)
	f.env.Storage.PutObject(objectKey, encryptedObject)

	completeInput := f.completeInput()
	if err := service.CompleteMultipartUploadSession(f.ctx, "22222222-2222-4222-8222-222222222222", started.UploadSessionID, completeInput); !errors.Is(err, ErrNotFound) {
		t.Fatalf("completion by a different user = %v, want ErrNotFound", err)
	}
	if err := service.CompleteMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID, completeInput); err != nil {
		t.Fatalf("complete upload: %v", err)
	}

	file, err := f.files.GetEncryptedFileRecordForUser(f.ctx, f.env.DB, started.FileID, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if file.UploadStatus != FileUploadComplete || file.ActualEncryptedSize != encryptedSize {
		t.Fatalf("completed file status/size = %q/%d, want %q/%d", file.UploadStatus, file.ActualEncryptedSize, FileUploadComplete, encryptedSize)
	}
	if string(file.EncryptedMetadata) != "metadata-ciphertext" || string(file.EncryptedFileKey) != "file-key-ciphertext" || string(file.EncryptedManifest) != "manifest-ciphertext" {
		t.Fatalf("completed file did not persist encrypted envelopes: %+v", file)
	}

	f.assertUsage(t, encryptedSize, 0)
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadComplete, UploadStatusCompleted)
	if err := service.CompleteMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID, completeInput); !errors.Is(err, ErrUploadCancelled) {
		t.Fatalf("repeated completion = %v, want ErrUploadCancelled", err)
	}
	f.assertUsage(t, encryptedSize, 0)
	storedObject, ok := f.env.Storage.GetObject(objectKey)
	if !ok || !bytes.Equal(storedObject, encryptedObject) {
		t.Fatalf("stored encrypted object exists=%t size=%d, want true/%d", ok, len(storedObject), encryptedSize)
	}
}

func TestEmptySinglePartUploadCompletesWithZeroStoredBytes(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	service := f.service(f.env.Storage)
	input := singlePartUploadInput()
	input.EncryptedSize = 0
	input.TotalChunks = 0
	started, validationErrors, err := service.StartMultipartUploadSession(f.ctx, f.userID, input)
	if err != nil || validationErrors.HasAny() {
		t.Fatalf("start empty file upload: validations=%v error=%v", validationErrors, err)
	}
	objectKey, err := storage.BuildObjectKey(f.userID, started.FileID)
	if err != nil {
		t.Fatal(err)
	}
	f.env.Storage.PutObject(objectKey, nil)
	if err := service.CompleteMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID, f.completeInput()); err != nil {
		t.Fatalf("complete empty file upload: %v", err)
	}
	file, err := f.files.GetEncryptedFileRecordForUser(f.ctx, f.env.DB, started.FileID, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if file.UploadStatus != FileUploadComplete || file.ActualEncryptedSize != 0 || file.ChunkCount != 0 {
		t.Fatalf("empty file state = status %q, size %d, chunks %d; want complete/0/0", file.UploadStatus, file.ActualEncryptedSize, file.ChunkCount)
	}
	f.assertUsage(t, 0, 0)
	storedObject, ok := f.env.Storage.GetObject(objectKey)
	if !ok || len(storedObject) != 0 {
		t.Fatalf("empty object exists=%t size=%d, want true/0", ok, len(storedObject))
	}
}
