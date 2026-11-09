package folders

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"arkive/core/models"
	trashrepo "arkive/core/repositories/trash"
	filessvc "arkive/core/services/files"
)

const maxTrashSelection = 1000

type trashCursor struct {
	DeletedAt string `json:"deletedAt"`
	Type      string `json:"type"`
	ID        string `json:"id"`
}

func (s *Service) ListTrash(ctx context.Context, input ListTrashInput) (ListTrashResult, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return ListTrashResult{}, ErrInvalidInput
	}
	if input.Limit <= 0 {
		input.Limit = defaultPageSize
	}
	if input.Limit > maxPageSize {
		input.Limit = maxPageSize
	}
	parentFolderID := strings.TrimSpace(input.ParentFolderID)
	if parentFolderID != "" {
		validatedParentID, err := validateUUIDValue(parentFolderID)
		if err != nil {
			return ListTrashResult{}, ErrInvalidInput
		}
		parentFolderID = validatedParentID
	}
	cutoff := time.Now().Add(-filessvc.TrashRetention)
	cursor, err := decodeTrashCursor(input.Cursor)
	if err != nil {
		return ListTrashResult{}, ErrInvalidInput
	}
	if strings.TrimSpace(input.SeekEntryID) != "" {
		if cursor != nil {
			return ListTrashResult{}, ErrInvalidInput
		}
		fileID, err := validateUUIDValue(input.SeekEntryID)
		if err != nil {
			return ListTrashResult{}, ErrInvalidInput
		}
		cursor, err = s.trashRepo.GetTrashFileCursor(ctx, s.db, userID, cutoff, parentFolderID, fileID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ListTrashResult{}, ErrEntryNotFound
			}
			return ListTrashResult{}, err
		}
	}

	var path []models.Folder
	if parentFolderID != "" && strings.TrimSpace(input.SeekEntryID) != "" {
		path, err = s.trashRepo.GetFolderPath(ctx, s.db, userID, parentFolderID)
		if err != nil {
			return ListTrashResult{}, err
		}
		if len(path) == 0 || path[0].ParentFolderID != nil {
			return ListTrashResult{}, ErrEntryNotFound
		}
	}
	entries, err := s.trashRepo.ListEntriesPage(ctx, s.db, userID, cutoff, parentFolderID, cursor, input.Limit+1)
	if err != nil {
		return ListTrashResult{}, err
	}
	hasMore := len(entries) > input.Limit
	if hasMore {
		entries = entries[:input.Limit]
	}
	result := ListTrashResult{Path: path, Entries: entries, HasMore: hasMore}
	if hasMore && len(entries) > 0 {
		encoded, err := encodeTrashCursor(entries[len(entries)-1])
		if err != nil {
			return ListTrashResult{}, err
		}
		result.NextCursor = &encoded
	}
	return result, nil
}

func (s *Service) RestoreTrash(ctx context.Context, input RestoreTrashInput) (RestoreTrashResult, error) {
	userID := strings.TrimSpace(input.UserID)
	selection, err := normalizeTrashSelection(input.FileIDs, input.FolderIDs, input.ContainerFolderIDs)
	if err != nil || userID == "" || selection.empty() {
		return RestoreTrashResult{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RestoreTrashResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cutoff := time.Now().Add(-filessvc.TrashRetention)
	result := RestoreTrashResult{}
	if len(selection.containerFolderIDs) > 0 {
		if err := s.lockTrashContainers(ctx, tx, userID, selection.containerFolderIDs); err != nil {
			return RestoreTrashResult{}, err
		}
		files, folders, err := s.trashRepo.RestoreRemainingTrashContents(
			ctx, tx, userID, cutoff, selection.containerFolderIDs,
		)
		if err != nil {
			return RestoreTrashResult{}, err
		}
		result.RestoredFiles = int(files)
		result.RestoredFolders = int(folders)
	} else {
		ancestors, err := s.lockTrashSelection(ctx, tx, userID, cutoff, selection)
		if err != nil {
			return RestoreTrashResult{}, err
		}
		if len(selection.folderIDs) > 0 {
			// Keep retention cleanup from purging contents while restoring their parent tree.
			blocked, err := s.trashRepo.LockRestorableFolderFiles(ctx, tx, userID, selection.folderIDs)
			if err != nil {
				return RestoreTrashResult{}, err
			}
			if blocked {
				return RestoreTrashResult{}, ErrNotFound
			}
		}

		if len(ancestors) > 0 {
			// Restore deleted parents too so the selected item remains at its original path.
			restored, err := s.trashRepo.RestoreTrashAncestors(ctx, tx, userID, ancestors)
			if err != nil {
				return RestoreTrashResult{}, err
			}
			if restored != int64(len(ancestors)) {
				return RestoreTrashResult{}, ErrNotFound
			}
			result.RestoredFolders += int(restored)
		}
		if len(selection.folderIDs) > 0 {
			files, folders, err := s.trashRepo.RestoreFolderTrees(ctx, tx, userID, cutoff, selection.folderIDs)
			if err != nil {
				return RestoreTrashResult{}, err
			}
			if folders < int64(len(selection.folderIDs)) {
				return RestoreTrashResult{}, ErrNotFound
			}
			result.RestoredFiles += int(files)
			result.RestoredFolders += int(folders)
		}
		if len(selection.fileIDs) > 0 {
			restored, err := s.trashRepo.RestoreFiles(ctx, tx, userID, cutoff, selection.fileIDs)
			if err != nil {
				return RestoreTrashResult{}, err
			}
			if restored != int64(len(selection.fileIDs)) {
				return RestoreTrashResult{}, ErrNotFound
			}
			result.RestoredFiles += int(restored)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RestoreTrashResult{}, err
	}
	return result, nil
}

func (s *Service) PermanentlyDeleteTrash(ctx context.Context, input PermanentlyDeleteTrashInput) (PermanentlyDeleteTrashResult, error) {
	userID := strings.TrimSpace(input.UserID)
	selection, err := normalizeTrashSelection(input.FileIDs, input.FolderIDs, input.ContainerFolderIDs)
	if err != nil || userID == "" || selection.empty() {
		return PermanentlyDeleteTrashResult{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PermanentlyDeleteTrashResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cutoff := time.Now().Add(-filessvc.TrashRetention)
	result := PermanentlyDeleteTrashResult{}
	if len(selection.containerFolderIDs) > 0 {
		if err := s.lockTrashContainers(ctx, tx, userID, selection.containerFolderIDs); err != nil {
			return PermanentlyDeleteTrashResult{}, err
		}
		queuedFiles, queuedFolders, err := s.trashRepo.QueuePermanentDeleteRemainingTrashContents(
			ctx, tx, userID, cutoff, selection.containerFolderIDs,
		)
		if err != nil {
			return PermanentlyDeleteTrashResult{}, err
		}
		result = PermanentlyDeleteTrashResult{QueuedFiles: int(queuedFiles), QueuedFolders: int(queuedFolders)}
	} else {
		if _, err := s.lockTrashSelection(ctx, tx, userID, cutoff, selection); err != nil {
			return PermanentlyDeleteTrashResult{}, err
		}

		// Persist intent first so failed object deletes can retry without making Trash restorable.
		queuedFiles, queuedFolders, err := s.trashRepo.QueuePermanentDelete(
			ctx, tx, userID, cutoff, selection.fileIDs, selection.folderIDs,
		)
		if err != nil {
			return PermanentlyDeleteTrashResult{}, err
		}
		if queuedFiles != int64(len(selection.fileIDs)) || queuedFolders != int64(len(selection.folderIDs)) {
			return PermanentlyDeleteTrashResult{}, ErrNotFound
		}
		result = PermanentlyDeleteTrashResult{QueuedFiles: int(queuedFiles), QueuedFolders: int(queuedFolders)}
	}
	if err := tx.Commit(ctx); err != nil {
		return PermanentlyDeleteTrashResult{}, err
	}
	return result, nil
}

func (s *Service) PurgeExpiredTrashFolders(ctx context.Context) (int, error) {
	cutoff := time.Now().Add(-filessvc.TrashRetention)
	count, err := s.trashRepo.MarkExpiredFoldersPurged(ctx, s.db, cutoff)
	return int(count), err
}

type trashSelection struct {
	fileIDs            []string
	folderIDs          []string
	containerFolderIDs []string
}

func (selection trashSelection) empty() bool {
	return len(selection.fileIDs)+len(selection.folderIDs)+len(selection.containerFolderIDs) == 0
}

func normalizeTrashSelection(fileIDs, folderIDs, containerFolderIDs []string) (trashSelection, error) {
	fileIDs = uniqueNonEmpty(fileIDs)
	folderIDs = uniqueNonEmpty(folderIDs)
	containerFolderIDs = uniqueNonEmpty(containerFolderIDs)
	if len(fileIDs)+len(folderIDs)+len(containerFolderIDs) > maxTrashSelection ||
		(len(containerFolderIDs) > 0 && len(fileIDs)+len(folderIDs) > 0) {
		return trashSelection{}, ErrInvalidInput
	}
	files, err := validateUUIDList(fileIDs)
	if err != nil {
		return trashSelection{}, err
	}
	folders, err := validateUUIDList(folderIDs)
	if err != nil {
		return trashSelection{}, err
	}
	containers, err := validateUUIDList(containerFolderIDs)
	if err != nil {
		return trashSelection{}, err
	}
	return trashSelection{fileIDs: files, folderIDs: folders, containerFolderIDs: containers}, nil
}

func (s *Service) lockTrashContainers(ctx context.Context, tx pgx.Tx, userID string, folderIDs []string) error {
	locked, err := s.trashRepo.LockActiveTrashContainers(ctx, tx, userID, folderIDs)
	if err != nil {
		return err
	}
	if locked != len(folderIDs) {
		return ErrNotFound
	}
	return nil
}

func (s *Service) lockTrashSelection(ctx context.Context, tx pgx.Tx, userID string, cutoff time.Time, selection trashSelection) ([]string, error) {
	lockedFolders, err := s.trashRepo.LockTrashFolders(ctx, tx, userID, cutoff, selection.folderIDs)
	if err != nil {
		return nil, err
	}
	if lockedFolders != len(selection.folderIDs) {
		return nil, ErrNotFound
	}
	lockedFiles, err := s.trashRepo.LockTrashFiles(ctx, tx, userID, cutoff, selection.fileIDs)
	if err != nil {
		return nil, err
	}
	if lockedFiles != len(selection.fileIDs) {
		return nil, ErrNotFound
	}
	ancestors, blocked, err := s.trashRepo.LockTrashAncestors(ctx, tx, userID, cutoff, selection.fileIDs, selection.folderIDs)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, ErrNotFound
	}
	return ancestors, nil
}

func decodeTrashCursor(encoded string) (*trashrepo.Cursor, error) {
	if strings.TrimSpace(encoded) == "" {
		return nil, nil
	}
	if len(encoded) > 512 {
		return nil, ErrInvalidInput
	}
	value, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	var cursor trashCursor
	if err := json.Unmarshal(value, &cursor); err != nil {
		return nil, err
	}
	if cursor.Type != "folder" && cursor.Type != "file" {
		return nil, ErrInvalidInput
	}
	id, err := validateUUIDValue(cursor.ID)
	if err != nil {
		return nil, err
	}
	deletedAt, err := time.Parse(time.RFC3339Nano, cursor.DeletedAt)
	if err != nil {
		return nil, err
	}
	return &trashrepo.Cursor{DeletedAt: deletedAt, Type: cursor.Type, ID: id}, nil
}

func encodeTrashCursor(entry models.TrashEntry) (string, error) {
	value, err := json.Marshal(trashCursor{
		DeletedAt: entry.DeletedAt.UTC().Format(time.RFC3339Nano),
		Type:      entry.Type,
		ID:        entry.ID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
