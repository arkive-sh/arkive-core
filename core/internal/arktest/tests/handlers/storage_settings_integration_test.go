//go:build integration

package handlers_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"arkive/core/handlers"
	"arkive/core/internal/arktest"
	"arkive/core/models"
	settingsrepo "arkive/core/repositories/settings"
	usersrepo "arkive/core/repositories/users"
	settingssvc "arkive/core/services/settings"
	appcontext "arkive/pkg/context"
)

func TestStorageSettingsAPIHidesSecretsAndPreservesThemWhenEditing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := arktest.NewEnvironment(t)
	repo := settingsrepo.New()
	const savedSecret = "saved-secret-value"
	const savedSessionToken = "saved-session-token"
	if err := repo.SaveStorageSettings(context.Background(), env.DB, models.StorageSettings{
		Provider:          "s3",
		MaxStorageBytes:   2 * 1024 * 1024 * 1024,
		S3AccessKeyID:     "saved-access-key",
		S3SecretAccessKey: savedSecret,
		S3SessionToken:    savedSessionToken,
		S3Bucket:          "arkive-test",
		S3Endpoint:        "https://storage.example.test",
		S3Region:          "auto",
	}); err != nil {
		t.Fatal(err)
	}

	service := settingssvc.NewService(env.DB, repo, usersrepo.New())
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-User") == "true" {
			c.Set(appcontext.UserKey, models.User{ID: "test-user"})
		}
	})
	router.GET("/settings/storage", handlers.APIStorageSettings(service))
	router.PUT("/settings/storage", handlers.APIUpdateStorageSettings(service))
	request := func(method, body string, authenticated bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/settings/storage", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if authenticated {
			req.Header.Set("X-Test-User", "true")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}

	if response := request(http.MethodGet, "", false); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	response := request(http.MethodGet, "", true)
	if response.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}
	if strings.Contains(response.Body.String(), savedSecret) || strings.Contains(response.Body.String(), savedSessionToken) {
		t.Fatal("GET response exposed a saved storage secret")
	}
	if !strings.Contains(response.Body.String(), "saved-access-key") || !strings.Contains(response.Body.String(), `"s3SecretAccessKeyConfigured":true`) {
		t.Fatalf("GET response omitted visible credentials or saved-secret status: %s", response.Body)
	}

	response = request(http.MethodPut, `{"storageGB":"3","s3AccessKeyID":"updated-access-key","s3Bucket":"updated-bucket","s3Endpoint":"https://storage.example.test","s3Region":"auto","clearSessionToken":true}`, true)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}
	if strings.Contains(response.Body.String(), savedSecret) || strings.Contains(response.Body.String(), savedSessionToken) {
		t.Fatal("PUT response exposed a saved storage secret")
	}

	settings, err := repo.GetStorageSettings(context.Background(), env.DB)
	if err != nil {
		t.Fatal(err)
	}
	if settings.S3SecretAccessKey != savedSecret {
		t.Fatal("editing other storage settings replaced the saved secret key")
	}
	if settings.S3SessionToken != "" {
		t.Fatal("clearing the saved session token did not take effect")
	}

	response = request(http.MethodPut, `{"storageGB":"3","s3AccessKeyID":"updated-access-key","s3SecretAccessKey":"replacement-secret","s3Bucket":"updated-bucket","s3Endpoint":"https://storage.example.test","s3Region":"auto"}`, true)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT replacement secret status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}
	settings, err = repo.GetStorageSettings(context.Background(), env.DB)
	if err != nil {
		t.Fatal(err)
	}
	if settings.S3SecretAccessKey != "replacement-secret" {
		t.Fatal("replacement secret key was not saved")
	}
	if settings.S3AccessKeyID != "updated-access-key" || settings.S3Bucket != "updated-bucket" || settings.MaxStorageBytes != 3*1024*1024*1024 {
		t.Fatalf("updated storage settings were not persisted: %+v", settings)
	}
}
