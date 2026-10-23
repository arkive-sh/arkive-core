package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"arkive/core/services/auth"
	"arkive/core/services/setup"
	appcontext "arkive/pkg/context"
	"arkive/pkg/cookies"
	"arkive/pkg/errs"
)

const (
	recoveryPendingCookie = "arkive_setup_recovery_pending"
	recoveryCookieTTL     = 15 * time.Minute
)

func WebRoot(authSvc *auth.Service, setupSvc *setup.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		initialized, err := setupSvc.IsInitialized(c.Request.Context())
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			c.Status(http.StatusInternalServerError)
			return
		}
		if !initialized {
			c.Redirect(http.StatusSeeOther, "/setup")
			return
		}

		if _, ok, err := appcontext.LoadUser(c, authSvc); err != nil {
			_ = c.Error(errs.WithStack(err))
			c.Status(http.StatusInternalServerError)
			return
		} else if ok {
			c.Redirect(http.StatusSeeOther, "/dashboard")
			return
		}

		ok, err := hasRecoveryPending(c, setupSvc)
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			c.Status(http.StatusInternalServerError)
			return
		}
		if ok {
			c.Redirect(http.StatusSeeOther, "/setup/recovery-key")
			return
		}

		c.Redirect(http.StatusSeeOther, "/login")
	}
}

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
