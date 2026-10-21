package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL           string
	Port                  string
	SessionTTL            time.Duration
	PasswordResetTokenTTL time.Duration
	BaseURL               string
	CORSAllowedOrigins    []string
	CookieSecret          string
	Env                   string
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

	return Config{
		DatabaseURL:           dsn,
		Port:                  addr,
		SessionTTL:            sessionTTL,
		PasswordResetTokenTTL: passwordResetTTL,
		BaseURL:               baseURL,
		CORSAllowedOrigins:    corsAllowedOrigins,
		CookieSecret:          cookieSecret,
		Env:                   env,
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
