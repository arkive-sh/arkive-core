package models

import "time"

type UploadStartResponse struct {
	UploadID         string
	FileID           string
	VaultID          string
	Mode             string
	UploadURL        string
	UploadSessionID  string
	ProviderUploadID string
	FileChunkSize    int64
	TotalChunks      int
	UploadPartSize   int64
	UploadPartCount  int
}

type UploadSession struct {
	ID               string
	FileID           string
	ProviderUploadID string
	EncryptedSize    int64
	UploadPartSize   int64
	UploadPartCount  int
	Status           string
	ExpiresAt        time.Time
}

type UploadPart struct {
	UploadSessionID string
	PartNumber      int
	ETag            string
	EncryptedHash   []byte
}
