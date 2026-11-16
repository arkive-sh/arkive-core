// Package settings loads and validates storage settings used by setup and uploads.
package settings

import (
	"context"
	"math"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"arkive/core/database"
	"arkive/core/models"
	settingsrepo "arkive/core/repositories/settings"
	usersrepo "arkive/core/repositories/users"
	"arkive/pkg/validation"
)

type Service struct {
	db           database.PgPool
	settingsRepo *settingsrepo.Repository
	usersRepo    *usersrepo.Repository
}

type StorageInput struct {
	StorageGB         string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3SessionToken    string
	S3Bucket          string
	S3Endpoint        string
	S3Region          string
	S3UsePathStyle    string
}

func NewService(db database.PgPool, settingsRepo *settingsrepo.Repository, usersRepo *usersrepo.Repository) *Service {
	return &Service{
		db:           db,
		settingsRepo: settingsRepo,
		usersRepo:    usersRepo,
	}
}

func (s *Service) StorageUsage(ctx context.Context, userID string) (models.StorageUsage, error) {
	usedBytes, reservedBytes, err := s.usersRepo.GetStorageUsage(ctx, s.db, userID)
	if err != nil {
		return models.StorageUsage{}, err
	}
	storage, err := s.settingsRepo.GetStorageSettings(ctx, s.db)
	if err != nil {
		return models.StorageUsage{}, err
	}
	return models.StorageUsage{
		UsedBytes:       usedBytes,
		ReservedBytes:   reservedBytes,
		MaxStorageBytes: storage.MaxStorageBytes,
	}, nil
}

func SaveStorageSettingsTx(ctx context.Context, tx pgx.Tx, settingsRepo *settingsrepo.Repository, settings models.StorageSettings) error {
	settings = NormalizeStorageSettings(settings)
	return settingsRepo.SaveStorageSettings(ctx, tx, settings)
}

func BuildStorageSettings(input StorageInput) (models.StorageSettings, validation.Errors) {
	validationErrors := validation.New()
	const bytesPerGB int64 = 1024 * 1024 * 1024
	storageGB := strings.TrimSpace(input.StorageGB)
	maxBytes := int64(0)
	if storageGB != "" {
		gb, err := strconv.ParseInt(storageGB, 10, 64)
		switch {
		case err != nil || gb < 0:
			validationErrors.Add("storage_gb", "storage limit must be 0 or a positive number")
		case gb > math.MaxInt64/bytesPerGB:
			validationErrors.Add("storage_gb", "storage limit is too large")
		case gb > 0:
			maxBytes = gb * bytesPerGB
		}
	}
	return NormalizeStorageSettings(models.StorageSettings{
		Provider:          "s3",
		MaxStorageBytes:   maxBytes,
		S3AccessKeyID:     strings.TrimSpace(input.S3AccessKeyID),
		S3SecretAccessKey: strings.TrimSpace(input.S3SecretAccessKey),
		S3SessionToken:    strings.TrimSpace(input.S3SessionToken),
		S3Bucket:          strings.TrimSpace(input.S3Bucket),
		S3Endpoint:        strings.TrimSpace(input.S3Endpoint),
		S3Region:          strings.TrimSpace(input.S3Region),
		S3UsePathStyle:    strings.TrimSpace(input.S3UsePathStyle) == "on" || strings.TrimSpace(input.S3UsePathStyle) == "true",
	}), validationErrors
}

func NormalizeStorageSettings(settings models.StorageSettings) models.StorageSettings {
	settings.Provider = strings.ToLower(strings.TrimSpace(settings.Provider))
	if settings.Provider == "" {
		settings.Provider = "s3"
	}
	settings.S3Region = strings.TrimSpace(settings.S3Region)
	if settings.Provider == "s3" && settings.S3Region == "" {
		settings.S3Region = "auto"
	}
	return settings
}

func ValidateStorageSettings(settings models.StorageSettings, validationErrors validation.Errors) {
	if settings.Provider != "s3" {
		validationErrors.Add("storage_provider", "S3-compatible storage is required")
	}
	if settings.S3AccessKeyID == "" {
		validationErrors.Add("s3_access_key_id", "access key is required")
	}
	if settings.S3SecretAccessKey == "" {
		validationErrors.Add("s3_secret_access_key", "secret key is required")
	}
	if settings.S3Bucket == "" {
		validationErrors.Add("s3_bucket", "bucket is required")
	}
	if settings.S3Endpoint == "" {
		validationErrors.Add("s3_endpoint", "endpoint is required")
	}
}

func DefaultUploadSettings() models.UploadSettings {
	return models.UploadSettings{
		MaxQueueItems:    300,
		StaleUploadHours: 1,
	}
}
