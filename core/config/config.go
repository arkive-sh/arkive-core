package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"arkive/core/opaque"
)

type Config struct {
	DatabaseURL             string
	Port                    string
	SessionTTL              time.Duration
	PasswordResetTokenTTL   time.Duration
	BaseURL                 string
	CORSAllowedOrigins      []string
	TrustedProxies          []string
	RateLimitRPM            int
	PublicShareRateLimitRPM int
	CookieSecret            string
	OpaqueServerSetup       string
	Env                     string
}

func Load() (Config, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	sessionTTL, err := parseDurationEnv("SESSION_TTL")
	if err != nil {
		return Config{}, err
	}
	passwordResetTTL, err := parseDurationEnvDefault("PASSWORD_RESET_TOKEN_TTL", "30m")
	if err != nil {
		return Config{}, err
	}
	cookieSecret := os.Getenv("COOKIE_SECRET")
	if cookieSecret == "" {
		return Config{}, errors.New("COOKIE_SECRET is required")
	}
	opaqueServerSetup := strings.TrimSpace(os.Getenv("OPAQUE_SERVER_SETUP"))
	if opaqueServerSetup == "" {
		return Config{}, errors.New("OPAQUE_SERVER_SETUP is required; generate one with `make opaque-setup`")
	}
	if err := opaque.ValidateSetup(opaqueServerSetup); err != nil {
		return Config{}, errors.New("OPAQUE_SERVER_SETUP is invalid; generate one with `make opaque-setup`")
	}

	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "prod"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("BASE_URL")), "/")
	if baseURL == "" {
		if env == "dev" {
			baseURL = "http://localhost" + addr
		} else {
			return Config{}, errors.New("BASE_URL is required")
		}
	}

	var corsAllowedOrigins []string
	for _, origin := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			corsAllowedOrigins = append(corsAllowedOrigins, origin)
		}
	}
	var trustedProxies []string
	for _, proxy := range strings.Split(os.Getenv("TRUSTED_PROXIES"), ",") {
		if proxy = strings.TrimSpace(proxy); proxy != "" {
			trustedProxies = append(trustedProxies, proxy)
		}
	}
	rateLimitRPM := 10
	if value := strings.TrimSpace(os.Getenv("RATE_LIMIT_RPM")); value != "" {
		rateLimitRPM, err = strconv.Atoi(value)
		if err != nil || rateLimitRPM < 0 {
			return Config{}, errors.New("RATE_LIMIT_RPM must be a non-negative integer")
		}
	}
	publicShareRateLimitRPM := 120
	if value := strings.TrimSpace(os.Getenv("PUBLIC_SHARE_RATE_LIMIT_RPM")); value != "" {
		publicShareRateLimitRPM, err = strconv.Atoi(value)
		if err != nil || publicShareRateLimitRPM < 0 {
			return Config{}, errors.New("PUBLIC_SHARE_RATE_LIMIT_RPM must be a non-negative integer")
		}
	}

	return Config{
		DatabaseURL:             dsn,
		Port:                    addr,
		SessionTTL:              sessionTTL,
		PasswordResetTokenTTL:   passwordResetTTL,
		BaseURL:                 baseURL,
		CORSAllowedOrigins:      corsAllowedOrigins,
		TrustedProxies:          trustedProxies,
		RateLimitRPM:            rateLimitRPM,
		PublicShareRateLimitRPM: publicShareRateLimitRPM,
		CookieSecret:            cookieSecret,
		OpaqueServerSetup:       opaqueServerSetup,
		Env:                     env,
	}, nil
}
func parseDurationEnv(key string) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return 0, errors.New(key + " is required")
	}
	return time.ParseDuration(value)
}

func parseDurationEnvDefault(key, fallback string) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		value = fallback
	}
	return time.ParseDuration(value)
}
