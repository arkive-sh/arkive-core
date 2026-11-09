package models

import "time"

type File struct {
	ID                  string
	UserID              string
	FolderID            *string
	EncryptedMetadata   []byte
	EncryptedFileKey    []byte
	EncryptedManifest   []byte
	EncryptionVersion   int16
	ChunkSize           int64
	ChunkCount          int
	ActualEncryptedSize int64
	EncryptedHash       []byte
	UploadStatus        string
	ThumbnailSizeBytes  int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ExpiresAt           *time.Time
	DeletedAt           *time.Time
	PurgedAt            *time.Time
	TombstonePurgedAt   *time.Time
	SearchScore         int64
	IsShared            bool
}
