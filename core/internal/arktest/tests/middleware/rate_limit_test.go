package middleware_test

import (
	. "arkive/core/middleware"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLimitRequestsPerIPReturnsRetryAfter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	called := 0
	router.Use(LimitRequestsPerIP(1))
	router.GET("/", func(c *gin.Context) {
		called++
		c.Status(http.StatusNoContent)
	})

	request := func() *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.0.2.10:1234"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}

	if response := request(); response.Code != http.StatusNoContent {
		t.Fatalf("first response status = %d, want %d", response.Code, http.StatusNoContent)
	}
	response := request()
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("limited response status = %d, want %d", response.Code, http.StatusTooManyRequests)
	}
	if retryAfter, err := strconv.Atoi(response.Header().Get("Retry-After")); err != nil || retryAfter < 1 {
		t.Fatalf("Retry-After = %q, want a positive number of seconds", response.Header().Get("Retry-After"))
	}
	if called != 1 {
		t.Fatalf("downstream handler ran %d times, want 1", called)
	}
}

func TestLimitRequestsPerIPDisabledAtZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	var calls atomic.Int32
	router.Use(LimitRequestsPerIP(0))
	router.GET("/", func(c *gin.Context) {
		calls.Add(1)
		c.Status(http.StatusNoContent)
	})

	for range 5 {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = "192.0.2.30:1234"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("disabled limiter response = %d, want %d", response.Code, http.StatusNoContent)
		}
	}
	if calls.Load() != 5 {
		t.Fatalf("handler calls = %d, want 5", calls.Load())
	}
}

func TestLimitRequestsPerIPUsesTrustedClientIPAndIgnoresSpoofedHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newRouter := func(trustedProxies []string) *gin.Engine {
		t.Helper()
		router := gin.New()
		if err := router.SetTrustedProxies(trustedProxies); err != nil {
			t.Fatal(err)
		}
		router.Use(LimitRequestsPerIP(1))
		router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		return router
	}
	request := func(router *gin.Engine, remoteAddr, forwardedFor string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remoteAddr
		if forwardedFor != "" {
			req.Header.Set("X-Forwarded-For", forwardedFor)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response.Code
	}

	untrusted := newRouter([]string{"10.0.0.0/8"})
	if code := request(untrusted, "198.51.100.20:1234", "203.0.113.1"); code != http.StatusNoContent {
		t.Fatalf("first direct request status = %d", code)
	}
	if code := request(untrusted, "198.51.100.20:1234", "203.0.113.2"); code != http.StatusTooManyRequests {
		t.Fatalf("spoofed forwarded IP bypassed the limiter: status = %d", code)
	}

	trusted := newRouter([]string{"10.0.0.0/8"})
	if code := request(trusted, "10.0.0.9:1234", "203.0.113.1"); code != http.StatusNoContent {
		t.Fatalf("first proxied request status = %d", code)
	}
	if code := request(trusted, "10.0.0.9:1234", "203.0.113.2"); code != http.StatusNoContent {
		t.Fatalf("distinct forwarded client should have its own bucket: status = %d", code)
	}
	if code := request(trusted, "10.0.0.9:1234", "203.0.113.1"); code != http.StatusTooManyRequests {
		t.Fatalf("same forwarded client bypassed its bucket: status = %d", code)
	}
}
