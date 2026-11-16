//go:build integration

package uploads_test

import (
	. "arkive/core/services/uploads"
	"errors"
	"testing"
)

func TestPresignedUploadPartIncludesChunkEnvelopeOverhead(t *testing.T) {
	f := newUploadIntegrationFixture(t)
	service := f.service(f.env.Storage)
	const (
		mib           = int64(1024 * 1024)
		chunkSize     = 4 * mib
		plaintextSize = 5 * mib
		encryptedSize = plaintextSize + 2*29
	)
	started, validationErrors, err := service.StartMultipartUploadSession(f.ctx, f.userID, MultipartUploadStartInput{
		EncryptedSize: encryptedSize, FileChunkSize: chunkSize, TotalChunks: 2,
		UploadPartSize: 8 * mib, UploadPartCount: 1, EncryptionVersion: 1,
	})
	if err != nil || validationErrors.HasAny() {
		t.Fatalf("start upload: validation=%v error=%v", validationErrors, err)
	}
	if _, err := service.PresignMultipartUploadParts(f.ctx, f.userID, started.UploadSessionID, []int{1}); err != nil {
		t.Fatal(err)
	}
	if got, ok := f.env.Storage.PresignedPartSize(1); !ok || got != encryptedSize {
		t.Fatalf("presigned encrypted part size = %d, recorded=%t; want %d", got, ok, encryptedSize)
	}
	if _, err := service.PresignMultipartUploadParts(f.ctx, f.userID, started.UploadSessionID, []int{2}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("presign out-of-range upload part error = %v, want ErrInvalidInput", err)
	}
}
