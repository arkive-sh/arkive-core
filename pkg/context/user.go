package context

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"arkive/core/models"
	"arkive/core/services/auth"
	"arkive/pkg/cookies"
)

const UserKey = "user"

func LoadUser(c *gin.Context, svc *auth.Service) (models.User, bool, error) {
	cookie, err := c.Request.Cookie(cookies.SessionName)
	if errors.Is(err, http.ErrNoCookie) {
		return models.User{}, false, nil
	}
	if err != nil {
		return models.User{}, false, err
	}
	if cookie.Value == "" {
		return models.User{}, false, nil
	}

	userID, err := svc.ValidateSession(c.Request.Context(), cookie.Value)
	if err != nil {
		if errors.Is(err, auth.ErrSessionNotFound) {
			cookies.ClearSession(c)
			return models.User{}, false, nil
		}
		return models.User{}, false, err
	}

	user, err := svc.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		cookies.ClearSession(c)
		return models.User{}, false, err
	}

	c.Set(UserKey, user)
	return user, true, nil
}

func UserFromContext(c *gin.Context) (models.User, bool) {
	userValue, ok := c.Get(UserKey)
	if !ok {
		return models.User{}, false
	}
	user, ok := userValue.(models.User)
	return user, ok
}
