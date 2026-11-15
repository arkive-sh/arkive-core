package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	settingssvc "arkive/core/services/settings"
	"arkive/pkg/apierror"
	appcontext "arkive/pkg/context"
	"arkive/pkg/errs"
)

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
