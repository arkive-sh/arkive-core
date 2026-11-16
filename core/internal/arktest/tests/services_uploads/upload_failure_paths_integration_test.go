//go:build integration

package uploads_test

import (
	. "arkive/core/services/uploads"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"arkive/core/models"
	"arkive/pkg/storage"
)

func TestMultipartUploadRetriesMissingPartAndPreservesPartOrder(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	service := f.service(f.env.Storage)
	chunkSize := int64(5 * 1024 * 1024)
	encryptedSize := chunkSize + 1 + 2*encryptedChunkEnvelopeOverheadBytes
	started, validationErrors, err := service.StartMultipartUploadSession(f.ctx, f.userID, MultipartUploadStartInput{
		EncryptedSize: encryptedSize, FileChunkSize: chunkSize, TotalChunks: 2,
		UploadPartSize: chunkSize, UploadPartCount: 2, EncryptionVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if validationErrors.HasAny() {
		t.Fatalf("valid multipart start failed validation: %v", validationErrors)
	}
	if err := service.RecordMultipartUploadPart(f.ctx, "22222222-2222-4222-8222-222222222222", started.UploadSessionID, UploadPartRecordInput{
		PartNumber: 1, EncryptedHash: "AA==", ETag: "unauthorized-part",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("record multipart part as another user = %v, want ErrNotFound", err)
	}
	if err := service.RecordMultipartUploadPart(f.ctx, f.userID, started.UploadSessionID, UploadPartRecordInput{
		PartNumber: 3, EncryptedHash: "AA==", ETag: "out-of-range-part",
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("record part beyond the upload part count = %v, want ErrInvalidInput", err)
	}
	urls, err := service.PresignMultipartUploadParts(f.ctx, f.userID, started.UploadSessionID, []int{2, 1})
	if err != nil || len(urls) != 2 {
		t.Fatalf("presign multipart parts returned %d URLs, error %v", len(urls), err)
	}

	firstPart := bytes.Repeat([]byte{1}, int(chunkSize+encryptedChunkEnvelopeOverheadBytes))
	secondPart := bytes.Repeat([]byte{2}, int(1+encryptedChunkEnvelopeOverheadBytes))
	f.env.Storage.PutPart(started.ProviderUploadID, 2, secondPart)
	if err := recordUploadPart(f, service, started.UploadSessionID, 2, "part-two"); err != nil {
		t.Fatal(err)
	}
	if err := service.CompleteMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID, f.completeInput()); !errors.Is(err, ErrPartsRequired) {
		t.Fatalf("completion with a missing part = %v, want ErrPartsRequired", err)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadUploading, UploadStatusActive)
	f.assertUsage(t, 0, reservedUploadSize(encryptedSize))

	f.env.Storage.PutPart(started.ProviderUploadID, 1, firstPart)
	if err := recordUploadPart(f, service, started.UploadSessionID, 1, "part-one"); err != nil {
		t.Fatal(err)
	}
	if err := service.CompleteMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID, f.completeInput()); err != nil {
		t.Fatalf("complete multipart upload after recording missing part: %v", err)
	}
	objectKey, err := storage.BuildObjectKey(f.userID, started.FileID)
	if err != nil {
		t.Fatal(err)
	}
	storedObject, ok := f.env.Storage.GetObject(objectKey)
	if !ok || !bytes.Equal(storedObject, append(firstPart, secondPart...)) {
		t.Fatalf("multipart object exists=%t size=%d, want ordered object size %d", ok, len(storedObject), encryptedSize)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadComplete, UploadStatusCompleted)
	f.assertUsage(t, encryptedSize, 0)
}

func TestOverQuotaUploadStartLeavesNoFileSessionOrReservation(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	service := f.service(f.env.Storage)
	if err := f.settings.SaveStorageSettings(f.ctx, f.env.DB, models.StorageSettings{Provider: "s3", MaxStorageBytes: 1}); err != nil {
		t.Fatal(err)
	}

	_, validationErrors, err := service.StartMultipartUploadSession(f.ctx, f.userID, singlePartUploadInput())
	var limitErr *StorageLimitExceededError
	if !errors.As(err, &limitErr) || validationErrors != nil {
		t.Fatalf("quota-limited start returned validations %v, error %v; want StorageLimitExceededError", validationErrors, err)
	}
	var fileCount, sessionCount int
	if err := f.env.DB.QueryRow(f.ctx, `SELECT count(*) FROM files WHERE user_id = $1`, f.userID).Scan(&fileCount); err != nil {
		t.Fatal(err)
	}
	if err := f.env.DB.QueryRow(f.ctx, `SELECT count(*) FROM upload_sessions`).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if fileCount != 0 || sessionCount != 0 {
		t.Fatalf("quota rejection left %d files and %d sessions, want none", fileCount, sessionCount)
	}
	f.assertUsage(t, 0, 0)
}

func TestOverQuotaMultipartStartDoesNotCreateRemoteUpload(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	if err := f.settings.SaveStorageSettings(f.ctx, f.env.DB, models.StorageSettings{
		Provider: "s3", MaxStorageBytes: 1,
	}); err != nil {
		t.Fatal(err)
	}
	provider := &countingMultipartStorage{Provider: f.env.Storage}
	service := f.service(provider)
	chunkSize := int64(5 * 1024 * 1024)
	input := MultipartUploadStartInput{
		EncryptedSize: chunkSize + 1 + 2*encryptedChunkEnvelopeOverheadBytes,
		FileChunkSize: chunkSize, TotalChunks: 2,
		UploadPartSize: chunkSize, UploadPartCount: 2, EncryptionVersion: 1,
	}

	_, validationErrors, err := service.StartMultipartUploadSession(f.ctx, f.userID, input)
	var limitErr *StorageLimitExceededError
	if !errors.As(err, &limitErr) || validationErrors != nil {
		t.Fatalf("quota-limited multipart start returned validations %v, error %v; want StorageLimitExceededError", validationErrors, err)
	}
	if provider.createCalls != 0 {
		t.Fatalf("remote multipart uploads created before quota rejection = %d, want 0", provider.createCalls)
	}
	var fileCount, sessionCount int
	if err := f.env.DB.QueryRow(f.ctx, `SELECT count(*) FROM files WHERE user_id = $1`, f.userID).Scan(&fileCount); err != nil {
		t.Fatal(err)
	}
	if err := f.env.DB.QueryRow(f.ctx, `SELECT count(*) FROM upload_sessions`).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if fileCount != 0 || sessionCount != 0 {
		t.Fatalf("quota rejection left %d files and %d sessions, want none", fileCount, sessionCount)
	}
	f.assertUsage(t, 0, 0)
}

func TestConcurrentUploadStartsCannotReserveMoreThanStorageLimit(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	service := f.service(f.env.Storage)
	if err := f.settings.SaveStorageSettings(f.ctx, f.env.DB, models.StorageSettings{Provider: "s3", MaxStorageBytes: reservedUploadSize(uploadTestEncryptedSize)}); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	type result struct {
		response models.UploadStartResponse
		err      error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			<-start
			response, validationErrors, err := service.StartMultipartUploadSession(f.ctx, f.userID, singlePartUploadInput())
			if validationErrors != nil && validationErrors.HasAny() {
				err = fmt.Errorf("unexpected upload validation errors: %v", validationErrors)
			}
			results <- result{response: response, err: err}
		}()
	}
	close(start)

	accepted := 0
	rejected := 0
	for range 2 {
		got := <-results
		if got.err == nil {
			accepted++
			continue
		}
		var limitErr *StorageLimitExceededError
		if errors.As(got.err, &limitErr) {
			rejected++
			continue
		}
		t.Fatalf("concurrent upload start returned unexpected error: %v", got.err)
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("concurrent upload starts accepted=%d rejected=%d; want one of each", accepted, rejected)
	}
	f.assertUsage(t, 0, reservedUploadSize(uploadTestEncryptedSize))
	var fileCount, sessionCount int
	if err := f.env.DB.QueryRow(f.ctx, `SELECT count(*) FROM files WHERE user_id = $1`, f.userID).Scan(&fileCount); err != nil {
		t.Fatal(err)
	}
	if err := f.env.DB.QueryRow(f.ctx, `SELECT count(*) FROM upload_sessions`).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if fileCount != 1 || sessionCount != 1 {
		t.Fatalf("concurrent quota result left %d files and %d sessions, want one each", fileCount, sessionCount)
	}
}

func TestOverQuotaUploadCompletionDeletesObjectAndReleasesReservation(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	service := f.service(f.env.Storage)
	started := f.startSinglePart(t, service)
	objectKey, err := storage.BuildObjectKey(f.userID, started.FileID)
	if err != nil {
		t.Fatal(err)
	}
	f.env.Storage.PutObject(objectKey, make([]byte, uploadTestEncryptedSize))
	if err := f.settings.SaveStorageSettings(f.ctx, f.env.DB, models.StorageSettings{Provider: "s3", MaxStorageBytes: 1}); err != nil {
		t.Fatal(err)
	}

	var limitErr *StorageLimitExceededError
	if err := service.CompleteMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID, f.completeInput()); !errors.As(err, &limitErr) {
		t.Fatalf("completion after quota reduction = %v, want StorageLimitExceededError", err)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadFailed, UploadStatusFailed)
	f.assertUsage(t, 0, 0)
	if _, ok := f.env.Storage.GetObject(objectKey); ok {
		t.Fatal("stored object remains after quota finalization failed")
	}
}

func TestOverQuotaCompletionRetriesCleanupAfterStorageDeleteFailure(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	storageFailure := &deleteFailureStorage{Provider: f.env.Storage, err: errors.New("temporary storage failure")}
	service := f.service(storageFailure)
	started := f.startSinglePart(t, service)
	objectKey, err := storage.BuildObjectKey(f.userID, started.FileID)
	if err != nil {
		t.Fatal(err)
	}
	f.env.Storage.PutObject(objectKey, make([]byte, uploadTestEncryptedSize))
	if err := f.settings.SaveStorageSettings(f.ctx, f.env.DB, models.StorageSettings{Provider: "s3", MaxStorageBytes: 1}); err != nil {
		t.Fatal(err)
	}

	var limitErr *StorageLimitExceededError
	if err := service.CompleteMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID, f.completeInput()); !errors.As(err, &limitErr) {
		t.Fatalf("completion after quota reduction = %v, want StorageLimitExceededError", err)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadUploading, UploadStatusActive)
	f.assertUsage(t, 0, reservedUploadSize(uploadTestEncryptedSize))
	if _, ok := f.env.Storage.GetObject(objectKey); !ok {
		t.Fatal("object was removed despite storage deletion failure")
	}

	storageFailure.err = nil
	if _, err := f.env.DB.Exec(f.ctx, `UPDATE upload_sessions SET expires_at = now() - interval '1 minute' WHERE id = $1`, started.UploadSessionID); err != nil {
		t.Fatal(err)
	}
	if cleaned, err := service.CleanupExpiredUploads(f.ctx); err != nil || cleaned != 1 {
		t.Fatalf("cleanup retry = %d, error %v; want 1, nil", cleaned, err)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadFailed, UploadStatusExpired)
	f.assertUsage(t, 0, 0)
	if _, ok := f.env.Storage.GetObject(objectKey); ok {
		t.Fatal("object remains after cleanup retry")
	}
}

func TestAbortAndExpiryDeleteObjectsAndReleaseReservations(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	service := f.service(f.env.Storage)

	aborted := f.startSinglePart(t, service)
	abortedKey, err := storage.BuildObjectKey(f.userID, aborted.FileID)
	if err != nil {
		t.Fatal(err)
	}
	abortedThumbnailKey, err := storage.BuildThumbnailObjectKey(f.userID, aborted.FileID)
	if err != nil {
		t.Fatal(err)
	}
	f.env.Storage.PutObject(abortedKey, make([]byte, uploadTestEncryptedSize))
	f.env.Storage.PutObject(abortedThumbnailKey, []byte("encrypted-thumbnail"))
	if err := service.AbortMultipartUploadSession(f.ctx, f.userID, aborted.UploadSessionID); err != nil {
		t.Fatalf("abort upload: %v", err)
	}
	f.assertStatuses(t, aborted.FileID, aborted.UploadSessionID, FileUploadAborted, UploadStatusAborted)
	f.assertUsage(t, 0, 0)
	if _, ok := f.env.Storage.GetObject(abortedKey); ok {
		t.Fatal("object remains after abort")
	}
	if _, ok := f.env.Storage.GetObject(abortedThumbnailKey); ok {
		t.Fatal("thumbnail remains after abort")
	}

	expired := f.startSinglePart(t, service)
	expiredKey, err := storage.BuildObjectKey(f.userID, expired.FileID)
	if err != nil {
		t.Fatal(err)
	}
	expiredThumbnailKey, err := storage.BuildThumbnailObjectKey(f.userID, expired.FileID)
	if err != nil {
		t.Fatal(err)
	}
	f.env.Storage.PutObject(expiredKey, make([]byte, uploadTestEncryptedSize))
	f.env.Storage.PutObject(expiredThumbnailKey, []byte("encrypted-thumbnail"))
	if _, err := f.env.DB.Exec(f.ctx, `UPDATE upload_sessions SET expires_at = now() - interval '1 minute' WHERE id = $1`, expired.UploadSessionID); err != nil {
		t.Fatal(err)
	}
	cleaned, err := service.CleanupExpiredUploads(f.ctx)
	if err != nil || cleaned != 1 {
		t.Fatalf("cleanup expired uploads = %d, error %v; want 1, nil", cleaned, err)
	}
	f.assertStatuses(t, expired.FileID, expired.UploadSessionID, FileUploadFailed, UploadStatusExpired)
	f.assertUsage(t, 0, 0)
	if _, ok := f.env.Storage.GetObject(expiredKey); ok {
		t.Fatal("object remains after stale-upload cleanup")
	}
	if _, ok := f.env.Storage.GetObject(expiredThumbnailKey); ok {
		t.Fatal("thumbnail remains after stale-upload cleanup")
	}
}

func TestAbortCompletedUploadPreservesStoredFileAndUsage(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	service := f.service(f.env.Storage)
	started := f.startSinglePart(t, service)
	objectKey, err := storage.BuildObjectKey(f.userID, started.FileID)
	if err != nil {
		t.Fatal(err)
	}
	f.env.Storage.PutObject(objectKey, make([]byte, uploadTestEncryptedSize))
	if err := service.CompleteMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID, f.completeInput()); err != nil {
		t.Fatalf("complete upload: %v", err)
	}

	if err := service.AbortMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID); !errors.Is(err, ErrUploadCancelled) {
		t.Fatalf("abort completed upload = %v, want ErrUploadCancelled", err)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadComplete, UploadStatusCompleted)
	f.assertUsage(t, uploadTestEncryptedSize, 0)
	if _, ok := f.env.Storage.GetObject(objectKey); !ok {
		t.Fatal("completed object was deleted by abort")
	}
}

func TestConcurrentUploadCompletionFinalizesReservationOnce(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	service := f.service(f.env.Storage)
	started := f.startSinglePart(t, service)
	objectKey, err := storage.BuildObjectKey(f.userID, started.FileID)
	if err != nil {
		t.Fatal(err)
	}
	f.env.Storage.PutObject(objectKey, make([]byte, uploadTestEncryptedSize))

	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			results <- service.CompleteMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID, f.completeInput())
		}()
	}
	ready.Wait()
	close(start)

	succeeded := 0
	refusedReplay := 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrUploadCancelled):
			refusedReplay++
		default:
			t.Fatalf("concurrent completion returned unexpected error: %v", err)
		}
	}
	if succeeded != 1 || refusedReplay != 1 {
		t.Fatalf("concurrent completions succeeded=%d refused=%d; want one each", succeeded, refusedReplay)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadComplete, UploadStatusCompleted)
	f.assertUsage(t, uploadTestEncryptedSize, 0)
	if _, ok := f.env.Storage.GetObject(objectKey); !ok {
		t.Fatal("completed object was removed by concurrent completion")
	}
}

func TestExpiredUploadCannotCompleteAndCleanupReleasesReservation(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	service := f.service(f.env.Storage)
	started := f.startSinglePart(t, service)
	objectKey, err := storage.BuildObjectKey(f.userID, started.FileID)
	if err != nil {
		t.Fatal(err)
	}
	f.env.Storage.PutObject(objectKey, make([]byte, uploadTestEncryptedSize))
	if _, err := f.env.DB.Exec(f.ctx, `UPDATE upload_sessions SET expires_at = now() - interval '1 minute' WHERE id = $1`, started.UploadSessionID); err != nil {
		t.Fatal(err)
	}

	if err := service.CompleteMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID, f.completeInput()); !errors.Is(err, ErrUploadCancelled) {
		t.Fatalf("completion after expiry = %v, want ErrUploadCancelled", err)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadUploading, UploadStatusActive)
	f.assertUsage(t, 0, reservedUploadSize(uploadTestEncryptedSize))

	if cleaned, err := service.CleanupExpiredUploads(f.ctx); err != nil || cleaned != 1 {
		t.Fatalf("cleanup after rejected completion = %d, error %v; want 1, nil", cleaned, err)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadFailed, UploadStatusExpired)
	f.assertUsage(t, 0, 0)
	if _, ok := f.env.Storage.GetObject(objectKey); ok {
		t.Fatal("expired object remains after cleanup")
	}
}

func TestAbortRetainsUploadAndReservationWhenObjectDeletionFails(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	storageFailure := &deleteFailureStorage{Provider: f.env.Storage, err: errors.New("temporary storage failure")}
	service := f.service(storageFailure)
	started := f.startSinglePart(t, service)
	objectKey, err := storage.BuildObjectKey(f.userID, started.FileID)
	if err != nil {
		t.Fatal(err)
	}
	f.env.Storage.PutObject(objectKey, make([]byte, uploadTestEncryptedSize))

	if err := service.AbortMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID); !errors.Is(err, storageFailure.err) {
		t.Fatalf("abort when object deletion fails = %v, want %v", err, storageFailure.err)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadUploading, UploadStatusActive)
	f.assertUsage(t, 0, reservedUploadSize(uploadTestEncryptedSize))
	if _, ok := f.env.Storage.GetObject(objectKey); !ok {
		t.Fatal("object was removed despite storage deletion failure")
	}

	storageFailure.err = nil
	if err := service.AbortMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID); err != nil {
		t.Fatalf("retry abort after storage recovers: %v", err)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadAborted, UploadStatusAborted)
	f.assertUsage(t, 0, 0)
	if _, ok := f.env.Storage.GetObject(objectKey); ok {
		t.Fatal("object remains after successful abort retry")
	}
}

func TestAbortAndCompletionSerializeBeforeDeletingUploadedObject(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	blockingStorage := &blockingDeleteStorage{
		Provider: f.env.Storage,
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
	}
	service := f.service(blockingStorage)
	started := f.startSinglePart(t, service)
	objectKey, err := storage.BuildObjectKey(f.userID, started.FileID)
	if err != nil {
		t.Fatal(err)
	}
	f.env.Storage.PutObject(objectKey, make([]byte, uploadTestEncryptedSize))

	abortDone := make(chan error, 1)
	go func() { abortDone <- service.AbortMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID) }()
	select {
	case <-blockingStorage.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("abort did not reach object deletion")
	}

	completeStarted := make(chan struct{})
	completeDone := make(chan error, 1)
	go func() {
		close(completeStarted)
		completeDone <- service.CompleteMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID, f.completeInput())
	}()
	<-completeStarted
	completeFinished := false
	var earlyCompleteErr error
	lockWaitObserved := false
	var lockQueryErr error
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case earlyCompleteErr = <-completeDone:
			completeFinished = true
		default:
		}
		if completeFinished {
			break
		}
		if err := f.env.DB.QueryRow(f.ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity
			WHERE pid <> pg_backend_pid() AND datname = current_database()
				AND wait_event_type = 'Lock' AND query ILIKE '%FOR UPDATE OF upload_sessions%'
		)`).Scan(&lockWaitObserved); err != nil {
			lockQueryErr = err
			break
		}
		if lockWaitObserved {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(blockingStorage.release)
	abortErr := <-abortDone
	completeErr := earlyCompleteErr
	if !completeFinished {
		completeErr = <-completeDone
	}
	if lockQueryErr != nil {
		t.Fatalf("observe completion waiting for upload lock: %v", lockQueryErr)
	}
	if !lockWaitObserved {
		t.Fatalf("completion did not wait on abort's upload lock (finished early: %t, error: %v)", completeFinished, earlyCompleteErr)
	}
	if abortErr != nil {
		t.Fatalf("abort upload: %v", abortErr)
	}
	if !errors.Is(completeErr, ErrUploadCancelled) {
		t.Fatalf("completion after abort = %v, want ErrUploadCancelled", completeErr)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadAborted, UploadStatusAborted)
	f.assertUsage(t, 0, 0)
	if _, ok := f.env.Storage.GetObject(objectKey); ok {
		t.Fatal("object remains after serialized abort")
	}
}

func TestExpiredUploadRetainsReservationWhenStorageDeleteFails(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	storageFailure := &deleteFailureStorage{Provider: f.env.Storage, err: errors.New("temporary storage failure")}
	service := f.service(storageFailure)
	started := f.startSinglePart(t, service)
	objectKey, err := storage.BuildObjectKey(f.userID, started.FileID)
	if err != nil {
		t.Fatal(err)
	}
	f.env.Storage.PutObject(objectKey, make([]byte, uploadTestEncryptedSize))
	if _, err := f.env.DB.Exec(f.ctx, `UPDATE upload_sessions SET expires_at = now() - interval '1 minute' WHERE id = $1`, started.UploadSessionID); err != nil {
		t.Fatal(err)
	}
	if cleaned, err := service.CleanupExpiredUploads(f.ctx); err != nil || cleaned != 0 {
		t.Fatalf("cleanup after storage failure = %d, error %v; want 0, nil", cleaned, err)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadUploading, UploadStatusActive)
	f.assertUsage(t, 0, reservedUploadSize(uploadTestEncryptedSize))
	if _, ok := f.env.Storage.GetObject(objectKey); !ok {
		t.Fatal("object was removed despite storage deletion failure")
	}
}

func TestMultipartCompletionFailureAbortsAndReleasesReservation(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	storageFailure := &completeFailureStorage{Provider: f.env.Storage, err: errors.New("multipart completion failed")}
	service := f.service(storageFailure)
	input := singlePartUploadInput()
	input.SinglePart = false
	started, validationErrors, err := service.StartMultipartUploadSession(f.ctx, f.userID, input)
	if err != nil || validationErrors.HasAny() {
		t.Fatalf("multipart start error=%v validations=%v", err, validationErrors)
	}
	f.env.Storage.PutPart(started.ProviderUploadID, 1, make([]byte, uploadTestEncryptedSize))
	if err := recordUploadPart(f, service, started.UploadSessionID, 1, "part-one"); err != nil {
		t.Fatal(err)
	}
	if err := service.CompleteMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID, f.completeInput()); err == nil || err.Error() != storageFailure.err.Error() {
		t.Fatalf("completion error = %v, want %v", err, storageFailure.err)
	}
	if storageFailure.abortCalls != 1 {
		t.Fatalf("storage abort calls = %d, want 1", storageFailure.abortCalls)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadFailed, UploadStatusFailed)
	f.assertUsage(t, 0, 0)
}

func TestMultipartCompletionRetainsReservationWhenStorageAbortFails(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	storageFailure := &completeFailureStorage{
		Provider: f.env.Storage,
		err:      errors.New("multipart completion failed"),
		abortErr: errors.New("multipart abort failed"),
	}
	service := f.service(storageFailure)
	input := singlePartUploadInput()
	input.SinglePart = false
	started, validationErrors, err := service.StartMultipartUploadSession(f.ctx, f.userID, input)
	if err != nil || validationErrors.HasAny() {
		t.Fatalf("multipart start error=%v validations=%v", err, validationErrors)
	}
	f.env.Storage.PutPart(started.ProviderUploadID, 1, make([]byte, uploadTestEncryptedSize))
	if err := recordUploadPart(f, service, started.UploadSessionID, 1, "part-one"); err != nil {
		t.Fatal(err)
	}
	if err := service.CompleteMultipartUploadSession(f.ctx, f.userID, started.UploadSessionID, f.completeInput()); !errors.Is(err, storageFailure.err) {
		t.Fatalf("completion with failed storage abort = %v, want %v", err, storageFailure.err)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadUploading, UploadStatusActive)
	f.assertUsage(t, 0, reservedUploadSize(uploadTestEncryptedSize))

	storageFailure.abortErr = nil
	if _, err := f.env.DB.Exec(f.ctx, `UPDATE upload_sessions SET expires_at = now() - interval '1 minute' WHERE id = $1`, started.UploadSessionID); err != nil {
		t.Fatal(err)
	}
	if cleaned, err := service.CleanupExpiredUploads(f.ctx); err != nil || cleaned != 1 {
		t.Fatalf("cleanup retry after storage abort failure = %d, error %v; want 1, nil", cleaned, err)
	}
	f.assertStatuses(t, started.FileID, started.UploadSessionID, FileUploadFailed, UploadStatusExpired)
	f.assertUsage(t, 0, 0)
}

type deleteFailureStorage struct {
	storage.Provider
	err error
}

type countingMultipartStorage struct {
	storage.Provider
	createCalls int
}

func (s *countingMultipartStorage) CreateMultipartUpload(ctx context.Context, key, contentType string) (string, error) {
	s.createCalls++
	return s.Provider.CreateMultipartUpload(ctx, key, contentType)
}

type blockingDeleteStorage struct {
	storage.Provider
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingDeleteStorage) DeleteObject(ctx context.Context, key string) error {
	s.once.Do(func() { close(s.entered) })
	select {
	case <-s.release:
		return s.Provider.DeleteObject(ctx, key)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *deleteFailureStorage) DeleteObject(ctx context.Context, key string) error {
	if s.err != nil {
		return s.err
	}
	return s.Provider.DeleteObject(ctx, key)
}

type completeFailureStorage struct {
	storage.Provider
	err        error
	abortErr   error
	abortCalls int
}

func (s *completeFailureStorage) CompleteMultipartUpload(context.Context, string, string, []storage.CompletedPart) error {
	return s.err
}

func (s *completeFailureStorage) AbortMultipartUpload(ctx context.Context, key, uploadID string) error {
	s.abortCalls++
	if s.abortErr != nil {
		return s.abortErr
	}
	return s.Provider.AbortMultipartUpload(ctx, key, uploadID)
}

func recordUploadPart(f *uploadIntegrationFixture, service *Service, sessionID string, partNumber int, hash string) error {
	return service.RecordMultipartUploadPart(f.ctx, f.userID, sessionID, UploadPartRecordInput{
		PartNumber:    partNumber,
		EncryptedHash: base64.StdEncoding.EncodeToString([]byte(hash)),
		ETag:          fmt.Sprintf("etag-%d", partNumber),
	})
}

var _ storage.Provider = (*deleteFailureStorage)(nil)
var _ storage.Provider = (*completeFailureStorage)(nil)
