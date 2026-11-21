package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"arkive/core/services/auth"
	totpservice "arkive/core/services/totp"
	"arkive/pkg/apierror"
	appcontext "arkive/pkg/context"
	"arkive/pkg/cookies"
	"arkive/pkg/errs"
)

type apiTOTPLoginRequest struct {
	Challenge      string `json:"challenge"`
	Code           string `json:"code"`
	RecoveryCode   string `json:"recoveryCode"`
	RememberDevice bool   `json:"rememberDevice"`
}

type apiTOTPCodeRequest struct {
	Code         string `json:"code"`
	RecoveryCode string `json:"recoveryCode"`
}

func APICompleteTOTPLogin(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request apiTOTPLoginRequest
		if err := c.ShouldBindJSON(&request); err != nil || len(request.Challenge) > 64 ||
			(strings.TrimSpace(request.Code) == "") == (strings.TrimSpace(request.RecoveryCode) == "") {
			apierror.InvalidPayload(c)
			return
		}
		result, err := svc.CompleteTOTPLogin(c.Request.Context(), request.Challenge, request.Code, request.RecoveryCode, request.RememberDevice, c.ClientIP())
		if err != nil {
			writeTOTPError(c, err, "Login verification failed")
			return
		}
		if result.TrustedDeviceToken == "" {
			cookies.ClearCustom(c, cookies.TOTPDeviceName)
		}
		writeLoginUnlock(c, result)
	}
}

func APIGetTOTPStatus(svc *totpservice.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := appcontext.UserFromContext(c)
		if !ok || user.ID == "" {
			apierror.Unauthorized(c)
			return
		}
		status, err := svc.Status(c.Request.Context(), user.ID)
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Could not load two-factor settings")
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func APIBeginTOTPEnrollment(svc *totpservice.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := appcontext.UserFromContext(c)
		if !ok || user.ID == "" {
			apierror.Unauthorized(c)
			return
		}
		setup, err := svc.BeginEnrollment(c.Request.Context(), user.ID, user.Username)
		if err != nil {
			if errors.Is(err, totpservice.ErrAlreadyEnabled) {
				apierror.Write(c, http.StatusConflict, "totp_already_enabled", err.Error(), nil)
				return
			}
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Could not start two-factor setup")
			return
		}
		c.JSON(http.StatusOK, setup)
	}
}

func APIConfirmTOTPEnrollment(svc *totpservice.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := appcontext.UserFromContext(c)
		if !ok || user.ID == "" {
			apierror.Unauthorized(c)
			return
		}
		var request apiTOTPCodeRequest
		if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Code) == "" {
			apierror.InvalidPayload(c)
			return
		}
		codes, err := svc.ConfirmEnrollment(c.Request.Context(), user.ID, request.Code)
		if err != nil {
			writeTOTPError(c, err, "Two-factor setup failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"recoveryCodes": codes})
	}
}

func APIDisableTOTP(svc *totpservice.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := appcontext.UserFromContext(c)
		if !ok || user.ID == "" {
			apierror.Unauthorized(c)
			return
		}
		var request apiTOTPCodeRequest
		if err := c.ShouldBindJSON(&request); err != nil ||
			(strings.TrimSpace(request.Code) == "") == (strings.TrimSpace(request.RecoveryCode) == "") {
			apierror.InvalidPayload(c)
			return
		}
		if err := svc.Disable(c.Request.Context(), user.ID, request.Code, request.RecoveryCode); err != nil {
			writeTOTPError(c, err, "Could not disable two-factor authentication")
			return
		}
		cookies.ClearCustom(c, cookies.TOTPDeviceName)
		c.Status(http.StatusNoContent)
	}
}

func writeTOTPError(c *gin.Context, err error, internalMessage string) {
	switch {
	case errors.Is(err, totpservice.ErrInvalidInput):
		apierror.InvalidPayload(c)
	case errors.Is(err, totpservice.ErrChallengeExpired):
		apierror.Write(c, http.StatusGone, "two_factor_challenge_expired", "This sign-in attempt expired. Start again.", nil)
	case errors.Is(err, totpservice.ErrInvalidCode):
		apierror.Write(c, http.StatusUnauthorized, "invalid_two_factor_code", "Verification code is invalid or expired", nil)
	case errors.Is(err, totpservice.ErrLocked):
		var cooldown *totpservice.LockedError
		if errors.As(err, &cooldown) {
			apierror.TooManyRequests(c, "two_factor_rate_limited", "Too many attempts. Try again later.", cooldown.RetryAfter)
		} else {
			apierror.TooManyRequests(c, "two_factor_rate_limited", "Too many attempts. Try again later.", time.Second)
		}
	case errors.Is(err, totpservice.ErrNotEnabled):
		apierror.Write(c, http.StatusConflict, "two_factor_not_enabled", err.Error(), nil)
	default:
		_ = c.Error(errs.WithStack(err))
		apierror.Internal(c, internalMessage)
	}
}
