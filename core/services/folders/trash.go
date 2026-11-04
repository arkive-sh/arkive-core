package folders

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
	cursor, err := decodeTrashCursor(input.Cursor)
	if err != nil {
		return ListTrashResult{}, ErrInvalidInput
	}

	cutoff := time.Now().Add(-filessvc.TrashRetention)
	entries, err := s.trashRepo.ListEntriesPage(ctx, s.db, userID, cutoff, parentFolderID, cursor, input.Limit+1)
	if err != nil {
		return ListTrashResult{}, err
	}
	hasMore := len(entries) > input.Limit
	if hasMore {
		entries = entries[:input.Limit]
	}
	result := ListTrashResult{Entries: entries, HasMore: hasMore}
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
	fileIDs, folderIDs, err := normalizeTrashSelection(input.FileIDs, input.FolderIDs)
	containerFolderIDs := uniqueNonEmpty(input.ContainerFolderIDs)
	selectedEntryCount := len(fileIDs) + len(folderIDs)
	selectionCount := selectedEntryCount + len(containerFolderIDs)
	if selectionCount > maxTrashSelection {
		return RestoreTrashResult{}, ErrInvalidInput
	}
	containerFolderIDs, containerErr := validateUUIDList(containerFolderIDs)
	if err != nil || containerErr != nil || userID == "" || selectionCount == 0 {
		return RestoreTrashResult{}, ErrInvalidInput
	}
	if len(containerFolderIDs) > 0 && selectedEntryCount > 0 {
		return RestoreTrashResult{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RestoreTrashResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cutoff := time.Now().Add(-filessvc.TrashRetention)
	if len(containerFolderIDs) > 0 {
		locked, err := s.trashRepo.LockActiveTrashContainers(ctx, tx, userID, containerFolderIDs)
		if err != nil {
			return RestoreTrashResult{}, err
		}
		if locked != len(containerFolderIDs) {
			return RestoreTrashResult{}, ErrNotFound
		}
		files, folders, err := s.trashRepo.RestoreTrashContainerContents(ctx, tx, userID, cutoff, containerFolderIDs)
		if err != nil {
			return RestoreTrashResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return RestoreTrashResult{}, err
		}
		return RestoreTrashResult{RestoredFiles: int(files), RestoredFolders: int(folders)}, nil
	}

	fileCount, folderCount, err := s.trashRepo.CountTrashEntries(ctx, tx, userID, cutoff, fileIDs, folderIDs)
	if err != nil {
		return RestoreTrashResult{}, err
	}
	if fileCount != len(fileIDs) || folderCount != len(folderIDs) {
		return RestoreTrashResult{}, ErrNotFound
	}
	lockedFolders, err := s.trashRepo.LockTrashFolders(ctx, tx, userID, cutoff, folderIDs)
	if err != nil {
		return RestoreTrashResult{}, err
	}
	if lockedFolders != len(folderIDs) {
		return RestoreTrashResult{}, ErrNotFound
	}
	lockedFiles, err := s.trashRepo.LockTrashFiles(ctx, tx, userID, cutoff, fileIDs)
	if err != nil {
		return RestoreTrashResult{}, err
	}
	if lockedFiles != len(fileIDs) {
		return RestoreTrashResult{}, ErrNotFound
	}
	ancestors, blocked, err := s.trashRepo.LockTrashAncestors(ctx, tx, userID, cutoff, fileIDs, folderIDs)
	if err != nil {
		return RestoreTrashResult{}, err
	}
	if blocked {
		return RestoreTrashResult{}, ErrNotFound
	}
	if len(folderIDs) > 0 {
		// Keep retention cleanup from purging contents while restoring their parent tree.
		blocked, err := s.trashRepo.LockRestorableFolderFiles(ctx, tx, userID, folderIDs)
		if err != nil {
			return RestoreTrashResult{}, err
		}
		if blocked {
			return RestoreTrashResult{}, ErrNotFound
		}
	}

	result := RestoreTrashResult{}
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
	if len(folderIDs) > 0 {
		files, folders, err := s.trashRepo.RestoreFolderTrees(ctx, tx, userID, cutoff, folderIDs)
		if err != nil {
			return RestoreTrashResult{}, err
		}
		if folders < int64(len(folderIDs)) {
			return RestoreTrashResult{}, ErrNotFound
		}
		result.RestoredFiles += int(files)
		result.RestoredFolders += int(folders)
	}
	if len(fileIDs) > 0 {
		restored, err := s.trashRepo.RestoreFiles(ctx, tx, userID, cutoff, fileIDs)
		if err != nil {
			return RestoreTrashResult{}, err
		}
		if restored != int64(len(fileIDs)) {
			return RestoreTrashResult{}, ErrNotFound
		}
		result.RestoredFiles += int(restored)
	}
	if err := tx.Commit(ctx); err != nil {
		return RestoreTrashResult{}, err
	}
	return result, nil
}

func (s *Service) PermanentlyDeleteTrash(ctx context.Context, input PermanentlyDeleteTrashInput) (PermanentlyDeleteTrashResult, error) {
	userID := strings.TrimSpace(input.UserID)
	fileIDs, folderIDs, err := normalizeTrashSelection(input.FileIDs, input.FolderIDs)
	if err != nil || userID == "" || len(fileIDs)+len(folderIDs) == 0 {
		return PermanentlyDeleteTrashResult{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PermanentlyDeleteTrashResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cutoff := time.Now().Add(-filessvc.TrashRetention)
	fileCount, folderCount, err := s.trashRepo.CountTrashEntries(ctx, tx, userID, cutoff, fileIDs, folderIDs)
	if err != nil {
		return PermanentlyDeleteTrashResult{}, err
	}
	if fileCount != len(fileIDs) || folderCount != len(folderIDs) {
		return PermanentlyDeleteTrashResult{}, ErrNotFound
	}
	lockedFolders, err := s.trashRepo.LockTrashFolders(ctx, tx, userID, cutoff, folderIDs)
	if err != nil {
		return PermanentlyDeleteTrashResult{}, err
	}
	if lockedFolders != len(folderIDs) {
		return PermanentlyDeleteTrashResult{}, ErrNotFound
	}
	lockedFiles, err := s.trashRepo.LockTrashFiles(ctx, tx, userID, cutoff, fileIDs)
	if err != nil {
		return PermanentlyDeleteTrashResult{}, err
	}
	if lockedFiles != len(fileIDs) {
		return PermanentlyDeleteTrashResult{}, ErrNotFound
	}
	_, blocked, err := s.trashRepo.LockTrashAncestors(ctx, tx, userID, cutoff, fileIDs, folderIDs)
	if err != nil {
		return PermanentlyDeleteTrashResult{}, err
	}
	if blocked {
		return PermanentlyDeleteTrashResult{}, ErrNotFound
	}

	// Persist intent first so failed object deletes can retry without making Trash restorable.
	queuedFiles, queuedFolders, err := s.trashRepo.RequestPurge(ctx, tx, userID, cutoff, fileIDs, folderIDs)
	if err != nil {
		return PermanentlyDeleteTrashResult{}, err
	}
	if queuedFiles != int64(len(fileIDs)) || queuedFolders != int64(len(folderIDs)) {
		return PermanentlyDeleteTrashResult{}, ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return PermanentlyDeleteTrashResult{}, err
	}
	return PermanentlyDeleteTrashResult{QueuedFiles: int(queuedFiles), QueuedFolders: int(queuedFolders)}, nil
}

func (s *Service) PurgeExpiredTrashFolders(ctx context.Context) (int, error) {
	cutoff := time.Now().Add(-filessvc.TrashRetention)
	count, err := s.trashRepo.MarkExpiredFoldersPurged(ctx, s.db, cutoff)
	return int(count), err
}

func normalizeTrashSelection(fileIDs, folderIDs []string) ([]string, []string, error) {
	fileIDs = uniqueNonEmpty(fileIDs)
	folderIDs = uniqueNonEmpty(folderIDs)
	if len(fileIDs)+len(folderIDs) > maxTrashSelection {
		return nil, nil, ErrInvalidInput
	}
	files, err := validateUUIDList(fileIDs)
	if err != nil {
		return nil, nil, err
	}
	folders, err := validateUUIDList(folderIDs)
	if err != nil {
		return nil, nil, err
	}
	return files, folders, nil
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
