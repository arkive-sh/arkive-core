package middleware

import (
	"github.com/gin-gonic/gin"

	"arkive/core/services/auth"
	"arkive/pkg/apierror"
	appcontext "arkive/pkg/context"
	"arkive/pkg/errs"
)

func RequireSessionJSON(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok, err := appcontext.LoadUser(c, svc)
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Session lookup failed")
			c.Abort()
			return
		}
		if !ok {
			apierror.Unauthorized(c)
			c.Abort()
			return
		}
		c.Set("user_id", user.ID)
		c.Next()
	}
}
