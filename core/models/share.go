package models

import "time"

type Share struct {
	ID                string
	ItemType          string
	FileID            string
	FolderID          string
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
	FileName                string
	FileContentType         string
	FileSizeBytes           int64
	FileUpdatedAt           time.Time
	FileVaultID             string
	FileFolderID            *string
	FileDeletedAt           *time.Time
	EncryptedFileMetadata   []byte
	EncryptedFileKey        []byte
	FolderVaultID           string
	FolderParentID          *string
	EncryptedFolderName     []byte
	EncryptedFolderMetadata []byte
	FolderDeletedAt         *time.Time
}

type PublicShareRecord struct {
	ShareID                  string
	Token                    string
	ItemType                 string
	FileID                   string
	FolderID                 string
	VaultID                  string
	EncryptedFolderName      []byte
	EncryptedFolderMetadata  []byte
	EncryptedFileKeyForShare []byte
	EncryptedMetadata        []byte
	EncryptedManifest        []byte
	EncryptedHash            []byte
	EncryptionVersion        int16
	ChunkSize                int64
	TotalChunks              int
	SourceURL                string
}

type PublicFolderShareEntry struct {
	ID                       string
	ItemType                 string
	ParentFolderID           *string
	VaultID                  string
	EncryptedName            []byte
	EncryptedMetadata        []byte
	EncryptedFileMetadata    []byte
	EncryptedFileKeyForShare []byte
	UpdatedAt                time.Time
	EncryptionVersion        int16
	ChunkSize                int64
	TotalChunks              int
	EncryptedHash            []byte
}
