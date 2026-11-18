package files

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"

	"arkive/pkg/storage"
)

const (
	deletedFilePurgeBatchSize = 100
	TrashRetention            = 30 * 24 * time.Hour
)

type deletedFileObjectKeys struct {
	keys []string
}

func (s *Service) PurgeDeletedFiles(ctx context.Context) (int, error) {
	total := 0
	// Each batch commits independently, so a deadline leaves remaining rows eligible for the next run.
	for {
		count, err := s.purgeDeletedFilesBatch(ctx)
		total += count
		if err != nil {
			return total, err
		}
		if count == 0 {
			return total, nil
		}
	}
}

func (s *Service) purgeDeletedFilesBatch(ctx context.Context) (int, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	files, err := s.fileRepo.ListDeletedPendingPurge(ctx, tx, time.Now().Add(-TrashRetention), deletedFilePurgeBatchSize)
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return 0, err
		}
		return 0, nil
	}

	fileIDs := make([]string, 0, len(files))
	uniqueStoredSizes := make(map[string]int64, len(files))
	for _, file := range files {
		fileIDs = append(fileIDs, file.ID)
		uniqueStoredSizes[file.ID] = file.TotalStoredBytes
	}
	versions, err := s.fileRepo.ListVersionStorageObjects(ctx, tx, fileIDs)
	if err != nil {
		return 0, err
	}
	keys := make([]string, 0, len(files)*2)
	seenKeys := make(map[string]struct{})
	fileKeys := make(map[string]deletedFileObjectKeys, len(files))
	for _, file := range files {
		objectSizes := map[string]int64{file.StorageObjectID: file.ThumbnailSizeBytes}
		for _, version := range versions[file.ID] {
			objectSizes[version.StorageObjectID] = version.ThumbnailSizeBytes
		}
		objectKeys := deletedFileObjectKeys{}
		for objectID, thumbnailSize := range objectSizes {
			objectKey, err := storage.BuildObjectKey(file.UserID, objectID)
			if err != nil {
				return 0, err
			}
			objectKeys.keys = append(objectKeys.keys, objectKey)
			if _, exists := seenKeys[objectKey]; !exists {
				keys = append(keys, objectKey)
				seenKeys[objectKey] = struct{}{}
			}
			if thumbnailSize <= 0 {
				continue
			}
			thumbnailKey, err := storage.BuildThumbnailObjectKey(file.UserID, objectID)
			if err != nil {
				return 0, err
			}
			objectKeys.keys = append(objectKeys.keys, thumbnailKey)
			if _, exists := seenKeys[thumbnailKey]; !exists {
				keys = append(keys, thumbnailKey)
				seenKeys[thumbnailKey] = struct{}{}
			}
		}
		fileKeys[file.ID] = objectKeys
	}

	failedKeys, err := s.storage.DeleteObjects(ctx, keys)
	if err != nil {
		return 0, fmt.Errorf("delete batch of %d file objects: %w", len(keys), err)
	}

	purgedIDs := make([]string, 0, len(files))
	for _, file := range files {
		objectKeys := fileKeys[file.ID]
		failed := false
		for _, key := range objectKeys.keys {
			if err := failedKeys[key]; err != nil {
				log.Printf("files: purge skipped file=%s user=%s object=%s: %v", file.ID, file.UserID, key, err)
				failed = true
			}
		}
		if failed {
			continue
		}
		purgedIDs = append(purgedIDs, file.ID)
	}
	log.Printf("files: purge batch candidates=%d objects=%d purged=%d failed=%d", len(files), len(keys), len(purgedIDs), len(files)-len(purgedIDs))
	if len(purgedIDs) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return 0, err
		}
		return 0, nil
	}

	purgedFiles, err := s.fileRepo.MarkFilesPurged(ctx, tx, purgedIDs)
	if err != nil {
		return 0, err
	}
	purgedFileIDs := make([]string, 0, len(purgedFiles))
	for _, file := range purgedFiles {
		purgedFileIDs = append(purgedFileIDs, file.ID)
		if err := s.storageRepo.DecreaseUsedStorage(ctx, tx, file.UserID, uniqueStoredSizes[file.ID]); err != nil {
			return 0, err
		}
	}
	if err := s.fileRepo.DeleteVersionsForFiles(ctx, tx, purgedFileIDs); err != nil {
		return 0, err
	}
	if err := s.shareRepo.DeleteLinksForFiles(ctx, tx, purgedFileIDs); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(purgedFiles), nil
}
