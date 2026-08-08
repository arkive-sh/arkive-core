package uploads

import (
	"fmt"
	"strings"
	"time"

	"arkive/pkg/validation"
)

const encryptedChunkEnvelopeOverheadBytes int64 = 41
const thumbnailMaxEncryptedBytes int64 = 150 * 1024
const thumbnailMimeWebP = "image/webp"

func validateUserID(userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", ErrUnauthorized
	}
	return userID, nil
}

func validateUploadID(uploadID string) (string, error) {
	uploadID = strings.TrimSpace(uploadID)
	if uploadID == "" {
		return "", ErrInvalidInput
	}
	return uploadID, nil
}

func validateOptionalFolderID(folderID *string) (*string, error) {
	if folderID == nil {
		return nil, nil
	}
	normalized, ok := validation.NormalizeUUID(*folderID)
	if !ok {
		return nil, ErrInvalidInput
	}
	return &normalized, nil
}

func expiresAtPtr(t time.Time) *time.Time {
	return &t
}

func encryptedFileSize(plaintextSize int64, chunkCount int) int64 {
	if plaintextSize <= 0 || chunkCount <= 0 {
		return 0
	}
	return plaintextSize + int64(chunkCount)*encryptedChunkEnvelopeOverheadBytes
}

func reservedUploadSize(plaintextSize int64, chunkCount int) int64 {
	return encryptedFileSize(plaintextSize, chunkCount) + thumbnailMaxEncryptedBytes
}

func expectedEncryptedPartSize(plaintextSize, chunkSize, partSize int64, partNumber int) (int64, error) {
	if plaintextSize <= 0 || chunkSize <= 0 || partSize <= 0 || partNumber <= 0 {
		return 0, fmt.Errorf("invalid upload part size")
	}
	start := int64(partNumber-1) * partSize
	if start >= plaintextSize {
		return 0, fmt.Errorf("upload part is out of range")
	}
	end := start + partSize
	if end > plaintextSize {
		end = plaintextSize
	}
	firstChunk := start / chunkSize
	lastChunk := (end + chunkSize - 1) / chunkSize
	return (end - start) + (lastChunk-firstChunk)*encryptedChunkEnvelopeOverheadBytes, nil
}
