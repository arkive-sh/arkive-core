package handlers

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"

	"github.com/gin-gonic/gin"

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
