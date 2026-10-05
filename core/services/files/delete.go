package files

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"arkive/core/models"
)

func (s *Service) DeleteFilesWithinTx(ctx context.Context, tx pgx.Tx, userID string, fileIDs []string) ([]models.File, error) {
	var err error
	userID, err = validateUserID(userID)
	if err != nil {
		return nil, err
	}
	uniqueIDs, err := normalizeDeleteFileIDs(fileIDs)
	if err != nil {
		return nil, err
	}
	if len(uniqueIDs) == 0 {
		return nil, ErrInvalidInput
	}
	return s.deleteFilesWithinTx(ctx, tx, userID, uniqueIDs)
}

func (s *Service) deleteFilesWithinTx(ctx context.Context, tx pgx.Tx, userID string, fileIDs []string) ([]models.File, error) {
	files := make([]models.File, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		file, getErr := s.fileRepo.GetFileForUser(ctx, tx, fileID, userID)
		if getErr != nil {
			if errors.Is(getErr, pgx.ErrNoRows) {
				return nil, ErrNotFound
			}
			return nil, getErr
		}

		switch file.UploadStatus {
		case "complete":
			if err := s.storageRepo.DecreaseUsedStorage(ctx, tx, userID, totalStoredSize(file)); err != nil {
				return nil, err
			}
		case "pending", "uploading":
			return nil, ErrUploadCancelled
		default:
			return nil, ErrNotFound
		}

		deleted, deleteErr := s.fileRepo.DeleteFileForUser(ctx, tx, fileID, userID)
		if deleteErr != nil {
			return nil, deleteErr
		}
		if !deleted {
			return nil, ErrNotFound
		}
		files = append(files, file)
	}
	return files, nil
}

func normalizeDeleteFileIDs(fileIDs []string) ([]string, error) {
	uniqueIDs := make([]string, 0, len(fileIDs))
	seen := make(map[string]struct{}, len(fileIDs))
	for _, rawID := range fileIDs {
		fileID, validateErr := validateUploadID(rawID)
		if validateErr != nil {
			return nil, validateErr
		}
		if _, exists := seen[fileID]; exists {
			continue
		}
		seen[fileID] = struct{}{}
		uniqueIDs = append(uniqueIDs, fileID)
	}
	return uniqueIDs, nil
}
