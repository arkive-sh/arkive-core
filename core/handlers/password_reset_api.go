package handlers

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"arkive/core/services/auth"
	"arkive/pkg/apierror"
	"arkive/pkg/errs"
)

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type completePasswordResetRequest struct {
	Token                    string `json:"token"`
	OpaqueRegistrationRecord string `json:"opaqueRegistrationRecord"`
	VaultSalt                string `json:"vaultSalt"`
	EncryptedMasterKey       string `json:"encryptedMasterKey"`
	RecoveryProof            string `json:"recoveryProof"`
}

type startPasswordResetRegistrationRequest struct {
	Token               string `json:"token"`
	RegistrationRequest string `json:"registrationRequest"`
}

func APIStartPasswordResetRegistration(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req startPasswordResetRegistrationRequest
		if err := c.ShouldBindJSON(&req); err != nil || len(req.RegistrationRequest) == 0 || len(req.RegistrationRequest) > 2048 {
			apierror.InvalidPayload(c)
			return
		}
		registrationRequest, err := base64.RawURLEncoding.DecodeString(req.RegistrationRequest)
		if err != nil {
			apierror.InvalidPayload(c)
			return
		}
		response, err := svc.StartPasswordResetRegistration(c.Request.Context(), req.Token, registrationRequest)
		if err != nil {
			if errors.Is(err, auth.ErrPasswordResetToken) {
				apierror.Write(c, http.StatusBadRequest, "password_reset_token_invalid", auth.ErrPasswordResetToken.Error(), nil)
				return
			}
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Password reset failed")
			return
		}
		defer response.Close()
		c.JSON(http.StatusOK, gin.H{"registrationResponse": base64.RawURLEncoding.EncodeToString(response.Bytes())})
	}
}

func APIForgotPassword(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req forgotPasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierror.InvalidPayload(c)
			return
		}

		result, err := svc.RequestPasswordReset(c.Request.Context(), req.Email)
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Password reset failed")
			return
		}

		// Arkive Core does not ship with email delivery.
		// When no mailer is configured, we return the `resetURL` directly so self-hosters can use
		// password recovery without extra infrastructure.
		// Completing this link still requires a valid recovery-key signature.
		c.JSON(http.StatusOK, gin.H{
			"ok":        true,
			"resetURL":  result.ResetURL,
			"expiresAt": result.ExpiresAt,
		})
	}
}

func APIPasswordResetVault(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimSpace(c.Query("token"))
		result, err := svc.LoadPasswordRecoveryVault(c.Request.Context(), token)
		if err != nil {
			switch err {
			case auth.ErrPasswordResetToken:
				apierror.Write(c, http.StatusBadRequest, "password_reset_token_invalid", auth.ErrPasswordResetToken.Error(), nil)
			case auth.ErrVaultNotConfigured:
				apierror.Write(c, http.StatusConflict, "vault_not_configured", "Vault is not configured", nil)
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Password reset failed")
			}
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"userID":                     result.UserID,
			"vaultSalt":                  base64.StdEncoding.EncodeToString(result.VaultSalt),
			"encryptedMasterKeyRecovery": base64.StdEncoding.EncodeToString(result.EncryptedMasterKeyRecovery),
		})
	}
}

func APICompletePasswordReset(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req completePasswordResetRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierror.InvalidPayload(c)
			return
		}

		registrationRecordValue := strings.TrimSpace(req.OpaqueRegistrationRecord)
		if len(registrationRecordValue) == 0 || len(registrationRecordValue) > 2048 {
			apierror.InvalidPayload(c)
			return
		}
		registrationRecord, err := base64.RawURLEncoding.DecodeString(registrationRecordValue)
		if err != nil || len(registrationRecord) == 0 {
			apierror.InvalidPayload(c)
			return
		}
		vaultSalt, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.VaultSalt))
		if err != nil {
			apierror.InvalidPayload(c)
			return
		}
		encryptedMasterKey, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.EncryptedMasterKey))
		if err != nil {
			apierror.InvalidPayload(c)
			return
		}
		recoveryProof, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.RecoveryProof))
		if err != nil {
			apierror.InvalidPayload(c)
			return
		}

		validationErrors, err := svc.CompletePasswordRecovery(
			c.Request.Context(),
			req.Token,
			registrationRecord,
			vaultSalt,
			encryptedMasterKey,
			recoveryProof,
		)
		if validationErrors != nil && validationErrors.HasAny() {
			apierror.Validation(c, validationErrors)
			return
		}
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Password reset failed")
			return
		}

		c.Status(http.StatusNoContent)
	}
}
