//go:build integration

package router_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"arkive/core/config"
	"arkive/core/internal/arktest"
	"arkive/core/router"
)

func TestProtectedAPIRoutesRejectRequestsWithoutASession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := arktest.NewEnvironment(t)
	r := router.New(env.DB, config.Config{CookieSecret: "test-cookie-secret"}, nil, nil, nil, nil)
	if err := r.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/me"},
		{http.MethodGet, "/api/settings/storage-usage"},
		{http.MethodGet, "/api/settings/storage"},
		{http.MethodPut, "/api/settings/storage"},
		{http.MethodPost, "/api/search"},
		{http.MethodPost, "/api/uploads/start"},
		{http.MethodGet, "/api/files/11111111-1111-4111-8111-111111111111/record"},
		{http.MethodGet, "/api/shares"},
		{http.MethodGet, "/api/folders/root/entries"},
		{http.MethodGet, "/api/trash/entries"},
		{http.MethodGet, "/api/sync/entries"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("response = %d, want %d: %s", response.Code, http.StatusUnauthorized, response.Body.String())
			}
		})
	}
}
