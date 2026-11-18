package files

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"arkive/core/models"
	"arkive/pkg/storage"
	"arkive/pkg/validation"
)

type FileVersionHistory struct {
	CurrentVersion int                  `json:"currentVersion"`
	Versions       []models.FileVersion `json:"versions"`
}

func (s *Service) ListFileVersions(ctx context.Context, userID, fileID string) (FileVersionHistory, error) {
	userID, err := validateUserID(userID)
	if err != nil {
		return FileVersionHistory{}, err
	}
	fileID, ok := validation.NormalizeUUID(strings.TrimSpace(fileID))
	if !ok {
		return FileVersionHistory{}, ErrInvalidInput
	}
	file, err := s.GetFileForDisplay(ctx, userID, fileID)
	if err != nil {
		return FileVersionHistory{}, err
	}
	versions, err := s.fileRepo.ListFileVersionsForUser(ctx, s.db, fileID, userID)
	if err != nil {
		return FileVersionHistory{}, err
	}
	return FileVersionHistory{CurrentVersion: file.VersionNumber, Versions: versions}, nil
}

func (s *Service) GetEncryptedFileVersionRecord(ctx context.Context, userID, fileID, versionID string) (EncryptedFileRecord, error) {
	userID, err := validateUserID(userID)
	if err != nil {
		return EncryptedFileRecord{}, err
	}
	fileID, ok := validation.NormalizeUUID(strings.TrimSpace(fileID))
	if !ok {
		return EncryptedFileRecord{}, ErrInvalidInput
	}
	versionID, ok = validation.NormalizeUUID(strings.TrimSpace(versionID))
	if !ok {
		return EncryptedFileRecord{}, ErrInvalidInput
	}
	version, err := s.fileRepo.GetFileVersionForUser(ctx, s.db, fileID, versionID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EncryptedFileRecord{}, ErrNotFound
		}
		return EncryptedFileRecord{}, err
	}
	objectKey, err := storage.BuildObjectKey(userID, version.StorageObjectID)
	if err != nil {
		return EncryptedFileRecord{}, err
	}
	sourceURL, err := s.storage.PresignDownload(ctx, objectKey, "ciphertext.bin", "inline", s.downloadExpire)
	if err != nil {
		return EncryptedFileRecord{}, err
	}
	return EncryptedFileRecord{
		FileID:            fileID,
		VaultID:           userID,
		FolderID:          version.FolderID,
		EncryptionVersion: version.EncryptionVersion,
		ChunkSize:         version.ChunkSize,
		TotalChunks:       version.ChunkCount,
		EncryptedHash:     base64.StdEncoding.EncodeToString(version.EncryptedHash),
		EncryptedMetadata: base64.StdEncoding.EncodeToString(version.EncryptedMetadata),
		EncryptedFileKey:  base64.StdEncoding.EncodeToString(version.EncryptedFileKey),
		EncryptedManifest: base64.StdEncoding.EncodeToString(version.EncryptedManifest),
		SourceURL:         sourceURL,
	}, nil
}

func (s *Service) RestoreFileVersion(ctx context.Context, userID, fileID, versionID string, folderID *string, encryptedMetadata, encryptedFileKey []byte, searchTokens []models.FileSearchToken) error {
	userID, err := validateUserID(userID)
	if err != nil {
		return err
	}
	fileID, ok := validation.NormalizeUUID(strings.TrimSpace(fileID))
	if !ok {
		return ErrInvalidInput
	}
	versionID, ok = validation.NormalizeUUID(strings.TrimSpace(versionID))
	if !ok || len(encryptedMetadata) == 0 || len(encryptedMetadata) > 1<<20 || len(encryptedFileKey) == 0 || len(encryptedFileKey) > 4096 {
		return ErrInvalidInput
	}
	searchTokens, err = NormalizeSearchTokens(searchTokens, MaxSearchTokensPerFile)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := s.fileRepo.LockFileForVersioning(ctx, tx, fileID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if current.UploadStatus != "complete" || isExpired(current.ExpiresAt) {
		return ErrNotFound
	}
	if (current.FolderID == nil) != (folderID == nil) || current.FolderID != nil && *current.FolderID != *folderID {
		return ErrConflict
	}
	if _, err := s.fileRepo.GetFileVersionForUser(ctx, tx, fileID, versionID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := s.fileRepo.SnapshotCurrentVersion(ctx, tx, fileID); err != nil {
		return err
	}
	if err := s.fileRepo.RestoreFileVersion(ctx, tx, fileID, versionID, encryptedMetadata, encryptedFileKey); err != nil {
		return err
	}
	if err := s.fileRepo.ReplaceSearchTokensForFile(ctx, tx, userID, userID, fileID, searchTokens); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) DeleteFileVersion(ctx context.Context, userID, fileID, versionID string) error {
	userID, err := validateUserID(userID)
	if err != nil {
		return err
	}
	fileID, ok := validation.NormalizeUUID(strings.TrimSpace(fileID))
	if !ok {
		return ErrInvalidInput
	}
	versionID, ok = validation.NormalizeUUID(strings.TrimSpace(versionID))
	if !ok {
		return ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := s.fileRepo.LockFileForVersioning(ctx, tx, fileID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	version, unreferenced, err := s.fileRepo.DeleteFileVersion(ctx, tx, fileID, versionID, userID)
	if err != nil {
		return err
	}
	if version.ID == "" {
		return ErrNotFound
	}
	if unreferenced {
		objectKey, err := storage.BuildObjectKey(userID, version.StorageObjectID)
		if err != nil {
			return err
		}
		if err := s.storage.DeleteObject(ctx, objectKey); err != nil {
			return err
		}
		if version.ThumbnailSizeBytes > 0 {
			thumbnailKey, err := storage.BuildThumbnailObjectKey(userID, version.StorageObjectID)
			if err != nil {
				return err
			}
			if err := s.storage.DeleteObject(ctx, thumbnailKey); err != nil {
				return err
			}
		}
		if err := s.storageRepo.DecreaseUsedStorage(ctx, tx, userID, totalStoredSize(models.File{
			ActualEncryptedSize: version.ActualEncryptedSize,
			ThumbnailSizeBytes:  version.ThumbnailSizeBytes,
		})); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
