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
	file      string
	thumbnail string
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

	keys := make([]string, 0, len(files)*2)
	fileKeys := make(map[string]deletedFileObjectKeys, len(files))
	for _, file := range files {
		objectKey, err := storage.BuildObjectKey(file.UserID, file.ID)
		if err != nil {
			return 0, err
		}
		keys = append(keys, objectKey)
		objectKeys := deletedFileObjectKeys{file: objectKey}
		var thumbnailKey string
		if file.ThumbnailSizeBytes > 0 {
			thumbnailKey, err = storage.BuildThumbnailObjectKey(file.UserID, file.ID)
			if err != nil {
				return 0, err
			}
			keys = append(keys, thumbnailKey)
			objectKeys.thumbnail = thumbnailKey
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
		if err := failedKeys[objectKeys.file]; err != nil {
			log.Printf("files: purge skipped file=%s user=%s object=%s: %v", file.ID, file.UserID, objectKeys.file, err)
			continue
		}
		if err := failedKeys[objectKeys.thumbnail]; objectKeys.thumbnail != "" && err != nil {
			log.Printf("files: purge skipped file=%s user=%s object=%s: %v", file.ID, file.UserID, objectKeys.thumbnail, err)
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
		if err := s.storageRepo.DecreaseUsedStorage(ctx, tx, file.UserID, totalStoredSize(file)); err != nil {
			return 0, err
		}
	}
	if err := s.shareRepo.DeleteLinksForFiles(ctx, tx, purgedFileIDs); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(purgedFiles), nil
}
