package models

import "time"

type TrashEntry struct {
	Type              string    `json:"type"`
	ID                string    `json:"id"`
	ParentFolderID    *string   `json:"parentFolderId,omitempty"`
	EncryptedName     []byte    `json:"encryptedName,omitempty"`
	EncryptedMetadata []byte    `json:"encryptedMetadata,omitempty"`
	EncryptedFileKey  []byte    `json:"encryptedFileKey,omitempty"`
	DeletedAt         time.Time `json:"deletedAt"`
	IsContainer       bool      `json:"isContainer,omitempty"`
}
