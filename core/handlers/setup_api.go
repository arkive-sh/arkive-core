package handlers

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	settingssvc "arkive/core/services/settings"
	"arkive/core/services/setup"
	"arkive/pkg/apierror"
	"arkive/pkg/errs"
	"arkive/pkg/validation"
)

type apiSetupStorageInput struct {
	Provider          string `json:"provider"`
	LocalPath         string `json:"localPath"`
	StorageGB         string `json:"storageGB"`
	S3AccessKeyID     string `json:"s3AccessKeyID"`
	S3SecretAccessKey string `json:"s3SecretAccessKey"`
	S3SessionToken    string `json:"s3SessionToken"`
	S3Bucket          string `json:"s3Bucket"`
	S3Endpoint        string `json:"s3Endpoint"`
	S3Region          string `json:"s3Region"`
	S3UsePathStyle    bool   `json:"s3UsePathStyle"`
}

type apiCreateSetupRequest struct {
	Name                     string               `json:"name"`
	Username                 string               `json:"username"`
	Email                    string               `json:"email"`
	OpaqueRegistrationRecord string               `json:"opaqueRegistrationRecord"`
	VaultSalt                string               `json:"vaultSalt"`
	EncryptedMasterKey       string               `json:"encryptedMasterKey"`
	Storage                  apiSetupStorageInput `json:"storage"`
}

type apiSetupOpaqueRegistrationRequest struct {
	Username            string `json:"username"`
	RegistrationRequest string `json:"registrationRequest"`
}

type apiSetupRecoveryRequest struct {
	ConfirmBackup              bool   `json:"confirmBackup"`
	EncryptedMasterKeyRecovery string `json:"encryptedMasterKeyRecovery"`
	RecoveryProofPublicKey     string `json:"recoveryProofPublicKey"`
}

func APISetupStatus(svc *setup.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		initialized, err := svc.IsInitialized(c.Request.Context())
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Could not read setup status")
			return
		}

		response := gin.H{"initialized": initialized, "recoveryPending": false}
		if initialized {
			pending, err := hasRecoveryPending(c, svc)
			if err != nil {
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Could not read setup status")
				return
			}
			response["recoveryPending"] = pending
			if pending {
				user, err := svc.GetRecoverySetupUser(c.Request.Context(), recoveryPendingToken(c))
				if err != nil {
					_ = c.Error(errs.WithStack(err))
					apierror.Internal(c, "Could not read setup status")
					return
				}
				response["userID"] = user.ID
			}
		}
		c.JSON(http.StatusOK, response)
	}
}

func APISetupOpaqueRegistrationStart(svc *setup.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apiSetupOpaqueRegistrationRequest
		if err := c.ShouldBindJSON(&req); err != nil || len(req.RegistrationRequest) == 0 || len(req.RegistrationRequest) > 2048 {
			apierror.InvalidPayload(c)
			return
		}
		registrationRequest, err := base64.RawURLEncoding.DecodeString(req.RegistrationRequest)
		if err != nil {
			apierror.InvalidPayload(c)
			return
		}
		response, err := svc.StartOpaqueRegistration(c.Request.Context(), req.Username, registrationRequest)
		if errors.Is(err, setup.ErrAlreadyInitialized) {
			apierror.Write(c, http.StatusConflict, "already_initialized", "This instance is already set up", nil)
			return
		}
		if errors.Is(err, setup.ErrInvalidRegistrationInput) {
			apierror.InvalidPayload(c)
			return
		}
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Registration could not be started")
			return
		}
		defer response.Close()
		c.JSON(http.StatusOK, gin.H{"registrationResponse": base64.RawURLEncoding.EncodeToString(response.Bytes())})
	}
}

func APICreateInitialSetup(svc *setup.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apiCreateSetupRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierror.InvalidPayload(c)
			return
		}

		storage, validationErrors := settingssvc.BuildStorageSettings(settingssvc.StorageInput{
			Provider:          req.Storage.Provider,
			LocalPath:         req.Storage.LocalPath,
			StorageGB:         req.Storage.StorageGB,
			S3AccessKeyID:     req.Storage.S3AccessKeyID,
			S3SecretAccessKey: req.Storage.S3SecretAccessKey,
			S3SessionToken:    req.Storage.S3SessionToken,
			S3Bucket:          req.Storage.S3Bucket,
			S3Endpoint:        req.Storage.S3Endpoint,
			S3Region:          req.Storage.S3Region,
		})
		if req.Storage.S3UsePathStyle {
			storage.S3UsePathStyle = true
		}
		if validationErrors.HasAny() {
			apierror.Validation(c, validationErrors)
			return
		}

		if len(req.OpaqueRegistrationRecord) == 0 || len(req.OpaqueRegistrationRecord) > 2048 {
			validationErrors.Add(validation.GeneralKey, "Password registration is missing or invalid")
			apierror.Validation(c, validationErrors)
			return
		}
		registrationRecord, err := base64.RawURLEncoding.DecodeString(req.OpaqueRegistrationRecord)
		if err != nil || len(registrationRecord) == 0 {
			validationErrors.Add(validation.GeneralKey, "Password registration is missing or invalid")
			apierror.Validation(c, validationErrors)
			return
		}

		vaultSalt, err := base64.StdEncoding.DecodeString(req.VaultSalt)
		if err != nil {
			validationErrors.Add(validation.GeneralKey, "Vault bootstrap failed. Reload the page and try again.")
			apierror.Validation(c, validationErrors)
			return
		}
		encryptedMasterKey, err := base64.StdEncoding.DecodeString(req.EncryptedMasterKey)
		if err != nil {
			validationErrors.Add(validation.GeneralKey, "Vault bootstrap failed. Reload the page and try again.")
			apierror.Validation(c, validationErrors)
			return
		}

		user, validationErrors, err := svc.CreateInitialAdmin(c.Request.Context(), setup.InitialAdminInput{
			Name:                     req.Name,
			Username:                 req.Username,
			Email:                    req.Email,
			OpaqueRegistrationRecord: registrationRecord,
			VaultSalt:                vaultSalt,
			EncryptedMasterKey:       encryptedMasterKey,
			Storage:                  storage,
			LocalStorageGB:           req.Storage.StorageGB,
		})
		if validationErrors.HasAny() {
			apierror.Validation(c, validationErrors)
			return
		}
		if err != nil {
			if errors.Is(err, setup.ErrAlreadyInitialized) {
				apierror.Write(c, http.StatusConflict, "already_initialized", "This instance is already set up", nil)
				return
			}
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Setup failed")
			return
		}

		if err := setRecoveryPending(c, svc, user.ID); err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Setup failed")
			return
		}
		c.JSON(http.StatusCreated, gin.H{"userID": user.ID})
	}
}

func APICompleteInitialSetup(svc *setup.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if pending, err := hasRecoveryPending(c, svc); err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Recovery setup failed")
			return
		} else if !pending {
			apierror.Unauthorized(c)
			return
		}

		var req apiSetupRecoveryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierror.InvalidPayload(c)
			return
		}
		if !req.ConfirmBackup {
			validationErrors := validation.New()
			validationErrors.Add(validation.GeneralKey, "You must confirm that the recovery key has been securely backed up.")
			apierror.Validation(c, validationErrors)
			return
		}

		encryptedMasterKeyRecovery, err := base64.StdEncoding.DecodeString(req.EncryptedMasterKeyRecovery)
		if err != nil || len(encryptedMasterKeyRecovery) == 0 {
			apierror.InvalidPayload(c)
			return
		}
		recoveryProofPublicKey, err := base64.StdEncoding.DecodeString(req.RecoveryProofPublicKey)
		if err != nil || len(recoveryProofPublicKey) != ed25519.PublicKeySize {
			apierror.InvalidPayload(c)
			return
		}

		if err := svc.SaveRecoveryWrappedMasterKey(c.Request.Context(), recoveryPendingToken(c), encryptedMasterKeyRecovery, recoveryProofPublicKey); err != nil {
			if errors.Is(err, setup.ErrInvalidRecoverySetup) {
				apierror.InvalidPayload(c)
				return
			}
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Recovery setup failed")
			return
		}
		if err := clearRecoveryPending(c, svc); err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Recovery setup failed")
			return
		}
		c.Status(http.StatusNoContent)
	}
}
