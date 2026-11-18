package uploads

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"

	"arkive/core/database"
	filerepo "arkive/core/repositories/files"
	folderrepo "arkive/core/repositories/folders"
	settingsrepo "arkive/core/repositories/settings"
	sharerepo "arkive/core/repositories/shares"
	storagerepo "arkive/core/repositories/storage"
	uploadrepo "arkive/core/repositories/uploads"
	usersrepo "arkive/core/repositories/users"
	"arkive/pkg/storage"
)

const staleUploadCleanupBatchSize = 100

const (
	FileStatusPending   = "pending"
	FileStatusUploading = "uploading"
	FileStatusComplete  = "complete"
	FileStatusFailed    = "failed"
	FileStatusAborted   = "aborted"
)

type Service struct {
	db            database.PgPool
	storageRepo   *storagerepo.Repository
	folderRepo    *folderrepo.Repository
	fileRepo      *filerepo.Repository
	settingsRepo  *settingsrepo.Repository
	uploadRepo    *uploadrepo.Repository
	userRepo      *usersrepo.Repository
	shareRepo     *sharerepo.Repository
	storage       storage.Provider
	uploadExpires time.Duration
}

type Config struct {
	UploadExpires       time.Duration
	DownloadExpire      time.Duration
	ShareDownloadExpire time.Duration
}

func NewService(
	db database.PgPool,
	storageRepo *storagerepo.Repository,
	folderRepo *folderrepo.Repository,
	fileRepo *filerepo.Repository,
	settingsRepo *settingsrepo.Repository,
	uploadRepo *uploadrepo.Repository,
	userRepo *usersrepo.Repository,
	shareRepo *sharerepo.Repository,
	storageProvider storage.Provider,
	cfg Config,
) *Service {
	return &Service{
		db:            db,
		storageRepo:   storageRepo,
		folderRepo:    folderRepo,
		fileRepo:      fileRepo,
		settingsRepo:  settingsRepo,
		uploadRepo:    uploadRepo,
		userRepo:      userRepo,
		shareRepo:     shareRepo,
		storage:       storageProvider,
		uploadExpires: cfg.UploadExpires,
	}
}

func isExpired(expiresAt *time.Time) bool {
	if expiresAt == nil {
		return false
	}
	return time.Now().After(*expiresAt)
}

func (s *Service) uploadExpiry(ctx context.Context) time.Duration {
	settings, err := s.settingsRepo.GetUploadSettings(ctx, s.db)
	if err != nil || settings.StaleUploadHours <= 0 {
		return s.uploadExpires
	}
	return time.Duration(settings.StaleUploadHours) * time.Hour
}

func (s *Service) CleanupExpiredUploads(ctx context.Context) (int, error) {
	stale, err := s.uploadRepo.ListExpiredActiveUploads(ctx, s.db, staleUploadCleanupBatchSize)
	if err != nil {
		return 0, err
	}
	cleaned := 0
	for _, candidate := range stale {
		tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return cleaned, err
		}
		upload, err := s.uploadRepo.GetExpiredUploadForUpdate(ctx, tx, candidate.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			_ = tx.Rollback(ctx)
			continue
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return cleaned, err
		}
		objectKey, keyErr := storage.BuildObjectKey(upload.UserID, upload.StorageObjectID)
		if keyErr != nil {
			_ = tx.Rollback(ctx)
			return cleaned, keyErr
		}
		thumbnailKey, err := storage.BuildThumbnailObjectKey(upload.UserID, upload.StorageObjectID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return cleaned, err
		}
		if err := s.storage.DeleteObject(ctx, thumbnailKey); err != nil {
			log.Printf("stale thumbnail cleanup failed file=%s: %v", upload.FileID, err)
			_ = tx.Rollback(ctx)
			continue
		}
		if upload.ProviderUploadID == "" {
			if err := s.storage.DeleteObject(ctx, objectKey); err != nil {
				log.Printf("stale upload object cleanup failed file=%s: %v", upload.FileID, err)
				_ = tx.Rollback(ctx)
				continue
			}
		} else if err := s.storage.AbortMultipartUpload(ctx, objectKey, upload.ProviderUploadID); err != nil {
			log.Printf("stale multipart cleanup failed session=%s: %v", upload.ID, err)
			_ = tx.Rollback(ctx)
			continue
		}
		claimed, err := s.uploadRepo.ExpireUploadSession(ctx, tx, upload.ID)
		updated := false
		if err == nil && claimed && upload.IsVersion {
			updated = true
		} else if err == nil && claimed {
			updated, err = s.fileRepo.UpdateEncryptedFileStatusIf(ctx, tx, upload.FileID, FileUploadFailed, []string{FileUploadUploading, FileUploadPending})
			if err == nil && !updated {
				err = ErrUploadFailed
			}
		}
		if err == nil && updated {
			var released bool
			released, err = s.storageRepo.ReleaseReservedStorage(ctx, tx, upload.UserID, reservedUploadSize(upload.EncryptedSize))
			if err == nil && !released {
				err = ErrUploadFailed
			}
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return cleaned, err
		}
		if err := tx.Commit(ctx); err != nil {
			return cleaned, err
		}
		if claimed {
			cleaned++
		}
	}
	return cleaned, nil
}
