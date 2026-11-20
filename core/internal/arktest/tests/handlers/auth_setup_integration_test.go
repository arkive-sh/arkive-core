//go:build integration

package handlers_test

import (
	. "arkive/core/handlers"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"

	"arkive/core/internal/arktest"
	"arkive/core/opaque"
	authrepo "arkive/core/repositories/auth"
	sessionrepo "arkive/core/repositories/session"
	settingsrepo "arkive/core/repositories/settings"
	totprepo "arkive/core/repositories/totp"
	usersrepo "arkive/core/repositories/users"
	"arkive/core/services/auth"
	"arkive/core/services/setup"
	totpservice "arkive/core/services/totp"
	"arkive/pkg/cookies"
)

func TestAuthAndSetupHandlersManageSessionsAndTrustedDevices(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("APP_ENV", "dev")
	env := arktest.NewEnvironment(t)
	arktest.InstallOpaqueLibrary(t)
	setupBuffer, err := opaque.GenerateSetup()
	if err != nil {
		t.Fatal(err)
	}
	serverSetup := string(setupBuffer.Bytes())
	setupBuffer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	const password = "handler test password"
	const email = "handler-user@example.test"
	const username = "handleruser"
	upload, err := arktest.OpaqueRegistrationUpload(ctx, serverSetup, username, password)
	if err != nil {
		t.Fatal(err)
	}
	registrationRecord, err := opaque.FinishRegistration(upload)
	if err != nil {
		t.Fatal(err)
	}
	user, err := arktest.CreateVerifiedUser(ctx, env.DB, "Handler Test User", username, email,
		registrationRecord.Bytes(), bytes.Repeat([]byte{6}, 16), []byte("encrypted-vault-key"))
	registrationRecord.Close()
	if err != nil {
		t.Fatal(err)
	}

	totpService := totpservice.NewService(env.DB, totprepo.New(), serverSetup)
	authService := auth.NewService(env.DB, authrepo.New(), sessionrepo.New(), usersrepo.New(), totpService, auth.Config{
		SessionTTL:            time.Hour,
		PasswordResetTokenTTL: time.Hour,
		BaseURL:               "https://arkive.example",
		OpaqueServerSetup:     serverSetup,
	})
	setupService := setup.NewService(env.DB, authrepo.New(), usersrepo.New(), settingsrepo.New(), serverSetup)
	router := gin.New()
	router.GET("/setup/status", APISetupStatus(setupService))
	router.POST("/setup/start", APISetupOpaqueRegistrationStart(setupService))
	router.POST("/setup", APICreateInitialSetup(setupService))
	router.POST("/setup/recovery-key", APICompleteInitialSetup(setupService))
	router.POST("/login/start", APIOpaqueLoginStart(authService))
	router.POST("/login/finish", APIOpaqueLoginFinish(authService))
	router.POST("/totp/verify", APICompleteTOTPLogin(authService))
	router.POST("/logout", APILogout(authService))
	router.POST("/password-reset/forgot", APIForgotPassword(authService))
	router.GET("/password-reset/vault", APIPasswordResetVault(authService))
	router.POST("/password-reset/start", APIStartPasswordResetRegistration(authService))
	router.POST("/password-reset/complete", APICompletePasswordReset(authService))

	request := func(method, path, body string, requestCookies ...*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		for _, cookie := range requestCookies {
			if cookie != nil {
				req.AddCookie(cookie)
			}
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/login/start", `{}`},
		{http.MethodPost, "/login/start", `{"email":"handler-user@example.test","startLoginRequest":"%%%"}`},
		{http.MethodPost, "/login/finish", `{}`},
		{http.MethodPost, "/login/finish", `{"attemptId":"attempt","finishLoginRequest":"%%%"}`},
		{http.MethodPost, "/setup/start", `{"username":"validuser","registrationRequest":"%%%"}`},
		{http.MethodPost, "/setup", `{}`},
		{http.MethodPost, "/password-reset/forgot", `not json`},
		{http.MethodPost, "/password-reset/start", `{}`},
		{http.MethodPost, "/password-reset/start", `{"token":"invalid","registrationRequest":"%%%"}`},
		{http.MethodPost, "/password-reset/complete", `{}`},
	} {
		if response := request(tc.method, tc.path, tc.body, nil); response.Code != http.StatusBadRequest {
			t.Errorf("%s %s response = %d, want %d: %s", tc.method, tc.path, response.Code, http.StatusBadRequest, response.Body.String())
		}
	}
	if response := request(http.MethodGet, "/setup/status", "", nil); response.Code != http.StatusOK {
		t.Fatalf("setup status response = %d, want %d", response.Code, http.StatusOK)
	}
	if response := request(http.MethodPost, "/setup/recovery-key", `{}`, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("recovery setup without pending setup response = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if response := request(http.MethodGet, "/password-reset/vault", "", nil); response.Code != http.StatusBadRequest {
		t.Fatalf("recovery vault request without a token = %d, want %d", response.Code, http.StatusBadRequest)
	}

	totpSetup, err := totpService.BeginEnrollment(ctx, user.ID, username)
	if err != nil {
		t.Fatal(err)
	}
	enrollmentCode, err := totp.GenerateCode(totpSetup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := totpService.ConfirmEnrollment(ctx, user.ID, enrollmentCode); err != nil {
		t.Fatal(err)
	}

	if response := request(http.MethodPost, "/password-reset/forgot", `{}`, nil); response.Code != http.StatusOK {
		t.Fatalf("forgot-password response = %d, want %d", response.Code, http.StatusOK)
	}
	invalidStart, err := arktest.OpaqueClient(ctx, "login-start", password, "", "")
	if err != nil {
		t.Fatal(err)
	}
	invalidStartBody, _ := json.Marshal(map[string]string{"email": email, "startLoginRequest": invalidStart.Request})
	invalidStartResponse := request(http.MethodPost, "/login/start", string(invalidStartBody), nil)
	var invalidLoginStart struct {
		AttemptID string `json:"attemptId"`
	}
	if invalidStartResponse.Code != http.StatusOK {
		t.Fatalf("login start for invalid proof response = %d", invalidStartResponse.Code)
	}
	if err := json.Unmarshal(invalidStartResponse.Body.Bytes(), &invalidLoginStart); err != nil {
		t.Fatal(err)
	}
	invalidFinishBody, _ := json.Marshal(map[string]string{
		"attemptId":          invalidLoginStart.AttemptID,
		"finishLoginRequest": base64.RawURLEncoding.EncodeToString([]byte("invalid OPAQUE finalization")),
	})
	if response := request(http.MethodPost, "/login/finish", string(invalidFinishBody), nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid OPAQUE finalization response = %d, want %d: %s", response.Code, http.StatusUnauthorized, response.Body.String())
	}

	startClient, err := arktest.OpaqueClient(ctx, "login-start", password, "", "")
	if err != nil {
		t.Fatal(err)
	}
	startBody, _ := json.Marshal(map[string]string{"email": email, "startLoginRequest": startClient.Request})
	startResponse := request(http.MethodPost, "/login/start", string(startBody), nil)
	if startResponse.Code != http.StatusOK {
		t.Fatalf("valid login start response = %d: %s", startResponse.Code, startResponse.Body.String())
	}
	var loginStart struct {
		AttemptID     string `json:"attemptId"`
		LoginResponse string `json:"loginResponse"`
	}
	if err := json.Unmarshal(startResponse.Body.Bytes(), &loginStart); err != nil {
		t.Fatal(err)
	}
	finishClient, err := arktest.OpaqueClient(ctx, "login-finish", password, startClient.State, loginStart.LoginResponse)
	if err != nil || !finishClient.Accepted {
		t.Fatalf("OPAQUE client rejected login response: result=%+v error=%v", finishClient, err)
	}
	finishBody, _ := json.Marshal(map[string]string{
		"attemptId": loginStart.AttemptID, "finishLoginRequest": finishClient.Request,
	})
	finishResponse := request(http.MethodPost, "/login/finish", string(finishBody), nil)
	if finishResponse.Code != http.StatusOK {
		t.Fatalf("valid login finish response = %d: %s", finishResponse.Code, finishResponse.Body.String())
	}
	var mfaResult struct {
		MFARequired bool   `json:"mfaRequired"`
		Challenge   string `json:"challenge"`
	}
	if err := json.Unmarshal(finishResponse.Body.Bytes(), &mfaResult); err != nil {
		t.Fatal(err)
	}
	if !mfaResult.MFARequired || mfaResult.Challenge == "" || len(finishResponse.Result().Cookies()) != 0 {
		t.Fatalf("login finish = %+v, cookies=%+v; want an MFA challenge without a session", mfaResult, finishResponse.Result().Cookies())
	}

	loginCode, err := totp.GenerateCode(totpSetup.Secret, time.Now().Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	verifyBody, _ := json.Marshal(map[string]any{
		"challenge": mfaResult.Challenge, "code": loginCode, "rememberDevice": true,
	})
	verifiedResponse := request(http.MethodPost, "/totp/verify", string(verifyBody), nil)
	if verifiedResponse.Code != http.StatusOK {
		t.Fatalf("TOTP verification response = %d: %s", verifiedResponse.Code, verifiedResponse.Body.String())
	}
	var loginResult struct {
		UserID             string `json:"userID"`
		Salt               string `json:"salt"`
		EncryptedMasterKey string `json:"encryptedMasterKey"`
	}
	if err := json.Unmarshal(verifiedResponse.Body.Bytes(), &loginResult); err != nil {
		t.Fatal(err)
	}
	if loginResult.UserID != user.ID || loginResult.Salt != base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{6}, 16)) || loginResult.EncryptedMasterKey != base64.StdEncoding.EncodeToString([]byte("encrypted-vault-key")) {
		t.Fatalf("login response returned incorrect user or vault credentials: %+v", loginResult)
	}
	loginCookies := verifiedResponse.Result().Cookies()
	if len(loginCookies) != 2 {
		t.Fatalf("login cookies = %+v, want session and trusted-device cookies", loginCookies)
	}
	var sessionCookie, deviceCookie *http.Cookie
	for _, cookie := range loginCookies {
		switch cookie.Name {
		case cookies.SessionName:
			sessionCookie = cookie
		case cookies.TOTPDeviceName:
			deviceCookie = cookie
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" || !sessionCookie.HttpOnly || sessionCookie.Path != "/" {
		t.Fatalf("login session cookie = %+v, want an HttpOnly session cookie", sessionCookie)
	}
	if deviceCookie == nil || deviceCookie.Value == "" || !deviceCookie.HttpOnly || deviceCookie.Path != "/" {
		t.Fatalf("remember-device cookie = %+v, want an HttpOnly trusted-device cookie", deviceCookie)
	}
	if got, err := authService.ValidateSession(ctx, sessionCookie.Value); err != nil || got != user.ID {
		t.Fatalf("session cookie does not identify logged-in user: user=%q error=%v", got, err)
	}

	logoutResponse := request(http.MethodPost, "/logout", "", sessionCookie, deviceCookie)
	if logoutResponse.Code != http.StatusNoContent {
		t.Fatalf("logout response = %d, want %d", logoutResponse.Code, http.StatusNoContent)
	}
	clearedCookies := logoutResponse.Result().Cookies()
	if len(clearedCookies) != 2 {
		t.Fatalf("logout cookies = %+v, want session and device cookies cleared", clearedCookies)
	}
	for _, cookie := range clearedCookies {
		if cookie.Value != "" || cookie.MaxAge != -1 {
			t.Fatalf("logout cookie = %+v, want cleared cookie", cookie)
		}
	}
	if _, err := authService.ValidateSession(ctx, sessionCookie.Value); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("logged-out cookie still validates: %v", err)
	}
}
