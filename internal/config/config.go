package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL         string
	HTTPAddr            string
	JWTSecret           string
	StripeSecretKey     string
	StripeWebhookSecret string
	StripeSuccessURL    string
	StripeCancelURL     string
	Provider            string
	ProxmoxURL          string
	ProxmoxTokenID      string
	ProxmoxTokenSecret  string
	ProxmoxNode         string
	ProxmoxStoragePool  string
	ProxmoxNetworkBridge string
	ProxmoxVLANID       int
	ProxmoxTemplates    map[string]int
	ProxyAddr           string
	ProxyAllowedDomains []string
	ScenarioRepoPath    string
	LogLevel            string
	JWTExpiry           time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:          getEnv("DATABASE_URL", "postgres://localhost:5432/painkiller?sslmode=disable"),
		HTTPAddr:             getEnv("HTTP_ADDR", ":8080"),
		JWTSecret:            getEnv("JWT_SECRET", ""),
		StripeSecretKey:      getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret:  getEnv("STRIPE_WEBHOOK_SECRET", ""),
		StripeSuccessURL:     getEnv("STRIPE_SUCCESS_URL", "http://localhost:3000/success"),
		StripeCancelURL:      getEnv("STRIPE_CANCEL_URL", "http://localhost:3000/cancel"),
		Provider:             getEnv("PROVIDER", "mock"),
		ProxmoxURL:           getEnv("PROXMOX_URL", ""),
		ProxmoxTokenID:       getEnv("PROXMOX_TOKEN_ID", ""),
		ProxmoxTokenSecret:   getEnv("PROXMOX_TOKEN_SECRET", ""),
		ProxmoxNode:          getEnv("PROXMOX_NODE", ""),
		ProxmoxStoragePool:   getEnv("PROXMOX_STORAGE_POOL", "local-lvm"),
		ProxmoxNetworkBridge: getEnv("PROXMOX_NETWORK_BRIDGE", "vmbr0"),
		ProxmoxVLANID:        getIntEnv("PROXMOX_VLAN_ID", 0),
		ProxmoxTemplates:     parseTemplateMap(getEnv("PROXMOX_TEMPLATES", "")),
		ProxyAddr:            getEnv("PROXY_ADDR", ""),
		ProxyAllowedDomains:  parseCSV(getEnv("PROXY_ALLOWED_DOMAINS", "kubernetes.io,k8s.io,helm.sh")),
		ScenarioRepoPath:     getEnv("SCENARIO_REPO_PATH", ""),
		LogLevel:             getEnv("LOG_LEVEL", "info"),
		JWTExpiry:            getDurationEnv("JWT_EXPIRY", 24*time.Hour),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	if cfg.Provider != "mock" && cfg.Provider != "proxmox" {
		return nil, fmt.Errorf("PROVIDER must be 'mock' or 'proxmox', got %q", cfg.Provider)
	}

	if cfg.Provider == "proxmox" {
		if cfg.ProxmoxURL == "" || cfg.ProxmoxTokenID == "" || cfg.ProxmoxTokenSecret == "" || cfg.ProxmoxNode == "" {
			return nil, fmt.Errorf("PROXMOX_URL, PROXMOX_TOKEN_ID, PROXMOX_TOKEN_SECRET, and PROXMOX_NODE are required when PROVIDER=proxmox")
		}
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

func getIntEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseTemplateMap(s string) map[string]int {
	result := make(map[string]int)
	if s == "" {
		return result
	}
	for _, pair := range strings.Split(s, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) != 2 {
			continue
		}
		vmid, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		result[strings.TrimSpace(parts[0])] = vmid
	}
	return result
}
