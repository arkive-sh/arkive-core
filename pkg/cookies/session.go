package cookies

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const SessionName = "arkive_session"

func SetSession(c *gin.Context, sessionID string, expiresAt time.Time) {
	SetCustom(c, SessionName, sessionID, expiresAt)
}

func ClearSession(c *gin.Context) {
	ClearCustom(c, SessionName)
}

func SetCustom(c *gin.Context, name, value string, expiresAt time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure(),
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})
}

func ClearCustom(c *gin.Context, name string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure(),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func secure() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "dev")
}
