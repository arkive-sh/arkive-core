package handlers

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"arkive/core/models"
	"arkive/core/services/auth"
	settingssvc "arkive/core/services/settings"
	"arkive/pkg/apierror"
	appcontext "arkive/pkg/context"
	"arkive/pkg/errs"
)

type apiRecoveryKeyUpdateRequest struct {
	EncryptedMasterKeyRecovery string `json:"encryptedMasterKeyRecovery"`
	RecoveryProofPublicKey     string `json:"recoveryProofPublicKey"`
}

type apiStorageSettingsRequest struct {
	StorageGB         string `json:"storageGB"`
	S3AccessKeyID     string `json:"s3AccessKeyID"`
	S3SecretAccessKey string `json:"s3SecretAccessKey"`
	S3SessionToken    string `json:"s3SessionToken"`
	ClearSessionToken bool   `json:"clearSessionToken"`
	S3Bucket          string `json:"s3Bucket"`
	S3Endpoint        string `json:"s3Endpoint"`
	S3Region          string `json:"s3Region"`
	S3UsePathStyle    bool   `json:"s3UsePathStyle"`
}

type apiStorageSettingsResponse struct {
	MaxStorageBytes             int64  `json:"maxStorageBytes"`
	S3AccessKeyID               string `json:"s3AccessKeyID"`
	S3SecretAccessKeyConfigured bool   `json:"s3SecretAccessKeyConfigured"`
	S3SessionTokenConfigured    bool   `json:"s3SessionTokenConfigured"`
	S3Bucket                    string `json:"s3Bucket"`
	S3Endpoint                  string `json:"s3Endpoint"`
	S3Region                    string `json:"s3Region"`
	S3UsePathStyle              bool   `json:"s3UsePathStyle"`
}

func storageSettingsResponse(settings models.StorageSettings) apiStorageSettingsResponse {
	return apiStorageSettingsResponse{
		MaxStorageBytes:             settings.MaxStorageBytes,
		S3AccessKeyID:               settings.S3AccessKeyID,
		S3SecretAccessKeyConfigured: settings.S3SecretAccessKey != "",
		S3SessionTokenConfigured:    settings.S3SessionToken != "",
		S3Bucket:                    settings.S3Bucket,
		S3Endpoint:                  settings.S3Endpoint,
		S3Region:                    settings.S3Region,
		S3UsePathStyle:              settings.S3UsePathStyle,
	}
}

func APIStorageSettings(settingsService *settingssvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := appcontext.UserFromContext(c); !ok {
			apierror.Unauthorized(c)
			return
		}

		settings, err := settingsService.StorageSettings(c.Request.Context())
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Could not load storage settings")
			return
		}
		c.JSON(http.StatusOK, storageSettingsResponse(settings))
	}
}

func APIUpdateStorageSettings(settingsService *settingssvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := appcontext.UserFromContext(c); !ok {
			apierror.Unauthorized(c)
			return
		}

		var request apiStorageSettingsRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			apierror.InvalidPayload(c)
			return
		}
		pathStyle := strconv.FormatBool(request.S3UsePathStyle)
		settings, validationErrors, err := settingsService.UpdateStorageSettings(c.Request.Context(), settingssvc.StorageInput{
			StorageGB:         request.StorageGB,
			S3AccessKeyID:     request.S3AccessKeyID,
			S3SecretAccessKey: request.S3SecretAccessKey,
			S3SessionToken:    request.S3SessionToken,
			ClearSessionToken: request.ClearSessionToken,
			S3Bucket:          request.S3Bucket,
			S3Endpoint:        request.S3Endpoint,
			S3Region:          request.S3Region,
			S3UsePathStyle:    pathStyle,
		})
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Could not update storage settings")
			return
		}
		if validationErrors != nil && validationErrors.HasAny() {
			apierror.Validation(c, validationErrors)
			return
		}
		c.JSON(http.StatusOK, storageSettingsResponse(settings))
	}
}

func APIStorageUsage(settingsService *settingssvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := appcontext.UserFromContext(c)
		if !ok || user.ID == "" {
			apierror.Unauthorized(c)
			return
		}

		usage, err := settingsService.StorageUsage(c.Request.Context(), user.ID)
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Could not load storage usage")
			return
		}
		c.JSON(http.StatusOK, usage)
	}
}

func APIUpdateRecoveryKey(authService *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := appcontext.UserFromContext(c)
		if !ok || user.ID == "" {
			apierror.Unauthorized(c)
			return
		}

		var request apiRecoveryKeyUpdateRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			apierror.InvalidPayload(c)
			return
		}
		encryptedMasterKeyRecovery, err := base64.StdEncoding.DecodeString(request.EncryptedMasterKeyRecovery)
		if err != nil || len(encryptedMasterKeyRecovery) == 0 || len(encryptedMasterKeyRecovery) > 1024 {
			apierror.InvalidPayload(c)
			return
		}
		recoveryProofPublicKey, err := base64.StdEncoding.DecodeString(request.RecoveryProofPublicKey)
		if err != nil || len(recoveryProofPublicKey) != ed25519.PublicKeySize {
			apierror.InvalidPayload(c)
			return
		}
		if err := authService.UpdateRecoveryKey(c.Request.Context(), user.ID, encryptedMasterKeyRecovery, recoveryProofPublicKey); err != nil {
			if err == auth.ErrInvalidInput {
				apierror.InvalidPayload(c)
				return
			}
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Could not update recovery key")
			return
		}
		c.Status(http.StatusNoContent)
	}
}
