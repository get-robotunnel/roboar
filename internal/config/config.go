// Package config loads runtime configuration from the environment.
package config

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	Port             string // HTTP listen port (default 8090)
	DatabaseURL      string // Postgres connection string (required)
	JWTSigningKey    string // HS256 signing key for owner JWTs (required)
	BaseURL          string // public base URL, e.g. https://reg.robotunnel.io/v1
	OfflineAfterSecs int    // heartbeat staleness window; default 60
}

// Load reads configuration from environment variables and applies defaults.
func Load() (*Config, error) {
	c := &Config{
		Port:             getenv("PORT", "8090"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		JWTSigningKey:    os.Getenv("JWT_SIGNING_KEY"),
		BaseURL:          getenv("REGISTRY_BASE_URL", "https://reg.robotunnel.io/v1"),
		OfflineAfterSecs: getenvInt("HEARTBEAT_OFFLINE_SECS", 60),
	}
	if c.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	if c.JWTSigningKey == "" {
		return nil, errors.New("JWT_SIGNING_KEY is required")
	}
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
