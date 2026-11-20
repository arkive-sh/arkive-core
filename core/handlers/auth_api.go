package handlers

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"arkive/core/services/auth"
	"arkive/pkg/apierror"
	"arkive/pkg/cookies"
	"arkive/pkg/errs"
)

type apiOpaqueLoginStartRequest struct {
	Email             string `json:"email"`
	StartLoginRequest string `json:"startLoginRequest"`
}

type apiOpaqueLoginFinishRequest struct {
	AttemptID          string `json:"attemptId"`
	FinishLoginRequest string `json:"finishLoginRequest"`
}

func APIOpaqueLoginStart(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apiOpaqueLoginStartRequest
		if err := c.ShouldBindJSON(&req); err != nil || len(req.StartLoginRequest) == 0 || len(req.StartLoginRequest) > 2048 {
			apierror.InvalidPayload(c)
			return
		}
		startRequest, err := base64.RawURLEncoding.DecodeString(req.StartLoginRequest)
		if err != nil {
			apierror.InvalidPayload(c)
			return
		}
		result, err := svc.StartOpaqueLogin(c.Request.Context(), req.Email, startRequest)
		if err != nil {
			if err == auth.ErrInvalidProtocolMessage {
				apierror.InvalidPayload(c)
				return
			}
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Login failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"attemptId": result.AttemptID, "loginResponse": result.LoginResponse})
	}
}

func APIOpaqueLoginFinish(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apiOpaqueLoginFinishRequest
		if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.AttemptID) == "" || len(req.FinishLoginRequest) == 0 || len(req.FinishLoginRequest) > 2048 {
			apierror.InvalidPayload(c)
			return
		}
		finalization, err := base64.RawURLEncoding.DecodeString(req.FinishLoginRequest)
		if err != nil {
			apierror.InvalidPayload(c)
			return
		}
		trustedDeviceToken, _ := c.Cookie(cookies.TOTPDeviceName)
		result, err := svc.FinishOpaqueLogin(c.Request.Context(), req.AttemptID, finalization, c.ClientIP(), trustedDeviceToken)
		if err != nil {
			switch err {
			case auth.ErrInvalidCredentials:
				apierror.Write(c, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password", nil)
			case auth.ErrVaultNotConfigured:
				apierror.Write(c, http.StatusConflict, "vault_not_configured", "Vault is not configured", nil)
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Login failed")
			}
			return
		}
		if result.MFARequired {
			if trustedDeviceToken != "" {
				cookies.ClearCustom(c, cookies.TOTPDeviceName)
			}
			response := gin.H{"mfaRequired": true, "challenge": result.MFAChallenge}
			if result.TOTPSetup != nil {
				response["totpSetup"] = result.TOTPSetup
			}
			c.JSON(http.StatusOK, response)
			return
		}
		writeLoginUnlock(c, result)
	}
}

func writeLoginUnlock(c *gin.Context, result auth.LoginUnlockResult) {
	cookies.SetSession(c, result.SessionToken, result.ExpiresAt)
	if result.TrustedDeviceToken != "" {
		cookies.SetCustom(c, cookies.TOTPDeviceName, result.TrustedDeviceToken, result.TrustedDeviceExpiry)
	}
	response := gin.H{
		"userID":             result.UserID,
		"salt":               base64.StdEncoding.EncodeToString(result.VaultSalt),
		"encryptedMasterKey": base64.StdEncoding.EncodeToString(result.EncryptedMasterKey),
	}
	if len(result.RecoveryCodes) > 0 {
		response["recoveryCodes"] = result.RecoveryCodes
	}
	c.JSON(http.StatusOK, response)
}

func APILogout(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionToken := ""
		cookie, err := c.Request.Cookie(cookies.SessionName)
		if err == nil && cookie.Value != "" {
			sessionToken = cookie.Value
		}

		if err := svc.LogoutSession(c.Request.Context(), sessionToken); err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Logout failed")
			return
		}

		cookies.ClearSession(c)
		cookies.ClearCustom(c, cookies.TOTPDeviceName)
		c.Status(http.StatusNoContent)
	}
}
