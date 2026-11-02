package models

import "time"

type Share struct {
	ID                string
	FileID            string
	OwnerUserID       string
	Token             string
	EncryptedShareKey []byte
	AllowPreview      bool
	AllowDownload     bool
	BurnAfterRead     bool
	AccessCount       int
	MaxAccessCount    *int
	PasswordHash      *string
	ExpiresAt         *time.Time
	Status            string
	RevokedAt         *time.Time
	ConsumedAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ShareWithFile struct {
	Share
	FileName        string
	FileContentType string
	FileSizeBytes   int64
	FileUpdatedAt   time.Time
}

type PublicShareRecord struct {
	ShareID                  string
	Token                    string
	FileID                   string
	VaultID                  string
	EncryptedFileKeyForShare []byte
	EncryptedMetadata        []byte
	EncryptedManifest        []byte
	EncryptedHash            []byte
	EncryptionVersion        int16
	ChunkSize                int64
	TotalChunks              int
	SourceURL                string
}
