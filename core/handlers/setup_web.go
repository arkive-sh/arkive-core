package handlers

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"arkive/core/services/setup"
	"arkive/pkg/cookies"
)

const (
	recoveryPendingCookie = "arkive_setup_recovery_pending"
	recoveryCookieTTL     = 15 * time.Minute
)

func recoveryPendingToken(c *gin.Context) string {
	cookie, err := c.Request.Cookie(recoveryPendingCookie)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func setRecoveryPending(c *gin.Context, svc *setup.Service, userID string) error {
	token, err := svc.IssueRecoverySetupToken(c.Request.Context(), userID, recoveryCookieTTL)
	if err != nil {
		return err
	}
	cookies.SetCustom(c, recoveryPendingCookie, token, time.Now().Add(recoveryCookieTTL))
	return nil
}

func hasRecoveryPending(c *gin.Context, svc *setup.Service) (bool, error) {
	return svc.HasValidRecoverySetupToken(c.Request.Context(), recoveryPendingToken(c))
}

func clearRecoveryPending(c *gin.Context, svc *setup.Service) error {
	token := recoveryPendingToken(c)
	if token != "" {
		if err := svc.ClearRecoverySetupToken(c.Request.Context(), token); err != nil {
			return err
		}
	}
	cookies.ClearCustom(c, recoveryPendingCookie)
	return nil
}
