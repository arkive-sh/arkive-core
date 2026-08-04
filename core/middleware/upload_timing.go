package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

func UploadTiming() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()

		log.Printf(
			"upload request method=%s path=%s session=%s status=%d in=%d out=%d duration=%s",
			c.Request.Method,
			c.Request.URL.Path,
			c.Param("id"),
			c.Writer.Status(),
			c.Request.ContentLength,
			c.Writer.Size(),
			time.Since(started),
		)
	}
}
