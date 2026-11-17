//go:build integration

package router_test

import (
	. "arkive/core/router"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"arkive/core/config"
	"arkive/core/internal/arktest"
)

func TestRateLimiterIsAppliedToAuthSetupAndPublicShareRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := arktest.NewEnvironment(t)
	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/setup/status"},
		{http.MethodPost, "/api/setup/opaque/registration/start"},
		{http.MethodPost, "/api/setup"},
		{http.MethodPost, "/api/setup/recovery-key"},
		{http.MethodPost, "/api/auth/opaque/login/start"},
		{http.MethodPost, "/api/auth/opaque/login/finish"},
		{http.MethodPost, "/api/auth/logout"},
		{http.MethodPost, "/api/auth/forgot-password"},
		{http.MethodGet, "/api/auth/password-reset/vault"},
		{http.MethodPost, "/api/auth/password-reset/opaque/start"},
		{http.MethodPost, "/api/auth/password-reset/complete"},
		{http.MethodPost, "/api/public/shares/token/unlock"},
		{http.MethodGet, "/api/public/shares/token"},
		{http.MethodPost, "/api/public/shares/token/consume"},
		{http.MethodGet, "/api/public/shares/token/folders/11111111-1111-4111-8111-111111111111/entries"},
		{http.MethodGet, "/api/public/shares/token/files/11111111-1111-4111-8111-111111111111"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			r := New(env.DB, config.Config{
				RateLimitRPM:            1,
				PublicShareRateLimitRPM: 1,
				CookieSecret:            "test-cookie-secret",
				OpaqueServerSetup:       "unused-in-this-test",
			}, nil, nil, nil, nil)
			if err := r.SetTrustedProxies(nil); err != nil {
				t.Fatal(err)
			}
			request := func() *httptest.ResponseRecorder {
				t.Helper()
				req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
				req.RemoteAddr = "192.0.2.41:4321"
				req.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				r.ServeHTTP(response, req)
				return response
			}
			if response := request(); response.Code == http.StatusTooManyRequests {
				t.Fatalf("first request was unexpectedly rate limited: %s", response.Body.String())
			}
			if response := request(); response.Code != http.StatusTooManyRequests {
				t.Fatalf("second request status = %d, want %d", response.Code, http.StatusTooManyRequests)
			}
		})
	}
}
