//go:build integration

package uploads_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	uploadrepo "arkive/core/repositories/uploads"
	"arkive/pkg/storage"
)

func TestVersionUploadKeepsCurrentFileUntilCompletionAndPreservesHistory(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	service := f.service(f.env.Storage)

	firstUpload := f.startSinglePart(t, service)
	firstObjectKey, err := storage.BuildObjectKey(f.userID, firstUpload.FileID)
	if err != nil {
		t.Fatal(err)
	}
	firstCiphertext := bytes.Repeat([]byte{0x11}, int(uploadTestEncryptedSize))
	f.env.Storage.PutObject(firstObjectKey, firstCiphertext)
	firstComplete := f.completeInput()
	if err := service.CompleteMultipartUploadSession(f.ctx, f.userID, firstUpload.UploadSessionID, firstComplete); err != nil {
		t.Fatalf("complete initial upload: %v", err)
	}

	versionInput := singlePartUploadInput()
	versionInput.ReplaceFileID = firstUpload.FileID
	versionUpload, validationErrors, err := service.StartMultipartUploadSession(f.ctx, f.userID, versionInput)
	if err != nil || validationErrors.HasAny() {
		t.Fatalf("start replacement upload: validations=%v error=%v", validationErrors, err)
	}
	if versionUpload.FileID != firstUpload.FileID {
		t.Fatalf("replacement file ID = %q, want stable logical file ID %q", versionUpload.FileID, firstUpload.FileID)
	}

	sessions := uploadrepo.New()
	versionSession, err := sessions.GetUploadSessionForUser(f.ctx, f.env.DB, versionUpload.UploadSessionID, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if !versionSession.IsVersion {
		t.Fatal("replacement upload session was not marked as a version")
	}
	if versionSession.StorageObjectID == firstUpload.FileID {
		t.Fatal("replacement upload reused the current storage object ID")
	}
	versionObjectKey, err := storage.BuildObjectKey(f.userID, versionSession.StorageObjectID)
	if err != nil {
		t.Fatal(err)
	}

	current, err := f.files.GetEncryptedFileRecordForUser(f.ctx, f.env.DB, firstUpload.FileID, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if current.VersionNumber != 1 || string(current.EncryptedMetadata) != "metadata-ciphertext" {
		t.Fatalf("current file changed before version completion: version=%d metadata=%q", current.VersionNumber, current.EncryptedMetadata)
	}
	versions, err := f.files.ListFileVersionsForUser(f.ctx, f.env.DB, firstUpload.FileID, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("history entries before replacement completes = %d, want 0", len(versions))
	}
	if _, ok := f.env.Storage.GetObject(firstObjectKey); !ok {
		t.Fatal("current encrypted object disappeared while replacement was uploading")
	}
	f.assertUsage(t, uploadTestEncryptedSize, reservedUploadSize(uploadTestEncryptedSize))

	secondCiphertext := bytes.Repeat([]byte{0x22}, int(uploadTestEncryptedSize))
	f.env.Storage.PutObject(versionObjectKey, secondCiphertext)
	secondComplete := f.completeInput()
	secondComplete.EncryptedMetadata = base64.StdEncoding.EncodeToString([]byte("metadata-v2-ciphertext"))
	secondComplete.EncryptedManifest = base64.StdEncoding.EncodeToString([]byte("manifest-v2-ciphertext"))
	if err := service.CompleteMultipartUploadSession(f.ctx, f.userID, versionUpload.UploadSessionID, secondComplete); err != nil {
		t.Fatalf("complete replacement upload: %v", err)
	}

	current, err = f.files.GetEncryptedFileRecordForUser(f.ctx, f.env.DB, firstUpload.FileID, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != firstUpload.FileID || current.VersionNumber != 2 || current.StorageObjectID != versionSession.StorageObjectID {
		t.Fatalf("current file identity/version/storage = %s/%d/%s, want %s/2/%s", current.ID, current.VersionNumber, current.StorageObjectID, firstUpload.FileID, versionSession.StorageObjectID)
	}
	if string(current.EncryptedMetadata) != "metadata-v2-ciphertext" || string(current.EncryptedManifest) != "manifest-v2-ciphertext" {
		t.Fatalf("current encrypted envelope was not replaced: metadata=%q manifest=%q", current.EncryptedMetadata, current.EncryptedManifest)
	}

	versions, err = f.files.ListFileVersionsForUser(f.ctx, f.env.DB, firstUpload.FileID, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("history entry count = %d, want 1", len(versions))
	}
	previous := versions[0]
	if previous.VersionNumber != 1 || previous.StorageObjectID != firstUpload.FileID || string(previous.EncryptedMetadata) != "metadata-ciphertext" || string(previous.EncryptedManifest) != "manifest-ciphertext" {
		t.Fatalf("previous encrypted version was not preserved: %+v", previous)
	}
	storedPrevious, ok := f.env.Storage.GetObject(firstObjectKey)
	if !ok || !bytes.Equal(storedPrevious, firstCiphertext) {
		t.Fatal("previous encrypted object was not retained unchanged")
	}
	storedCurrent, ok := f.env.Storage.GetObject(versionObjectKey)
	if !ok || !bytes.Equal(storedCurrent, secondCiphertext) {
		t.Fatal("new encrypted object is missing or has unexpected bytes")
	}
	f.assertUsage(t, 2*uploadTestEncryptedSize, 0)
}
