package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL         string
	HTTPAddr            string
	JWTSecret           string
	StripeSecretKey     string
	StripeWebhookSecret string
	ProxmoxURL          string
	ProxmoxTokenID      string
	ProxmoxTokenSecret  string
	ProxmoxNode         string
	ScenarioRepoPath    string
	LogLevel            string
	JWTExpiry           time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://localhost:5432/painkiller?sslmode=disable"),
		HTTPAddr:            getEnv("HTTP_ADDR", ":8080"),
		JWTSecret:           getEnv("JWT_SECRET", ""),
		StripeSecretKey:     getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret: getEnv("STRIPE_WEBHOOK_SECRET", ""),
		ProxmoxURL:          getEnv("PROXMOX_URL", ""),
		ProxmoxTokenID:      getEnv("PROXMOX_TOKEN_ID", ""),
		ProxmoxTokenSecret:  getEnv("PROXMOX_TOKEN_SECRET", ""),
		ProxmoxNode:         getEnv("PROXMOX_NODE", ""),
		ScenarioRepoPath:    getEnv("SCENARIO_REPO_PATH", ""),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		JWTExpiry:           getDurationEnv("JWT_EXPIRY", 24*time.Hour),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}
