package config

import (
	"os"
	"testing"
	"time"
)

func setEnvs(t *testing.T, envs map[string]string) {
	t.Helper()
	for k, v := range envs {
		t.Setenv(k, v)
	}
}

func TestLoadDefaults(t *testing.T) {
	os.Clearenv()
	t.Setenv("JWT_SECRET", "test-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.HTTPAddr != ":8080" {
		t.Errorf("expected HTTPAddr ':8080', got %q", cfg.HTTPAddr)
	}
	if cfg.Provider != "mock" {
		t.Errorf("expected Provider 'mock', got %q", cfg.Provider)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected LogLevel 'info', got %q", cfg.LogLevel)
	}
	if cfg.JWTExpiry != 24*time.Hour {
		t.Errorf("expected JWTExpiry 24h, got %v", cfg.JWTExpiry)
	}
}

func TestLoadMissingJWTSecret(t *testing.T) {
	os.Clearenv()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing JWT_SECRET")
	}
}

func TestLoadInvalidProvider(t *testing.T) {
	os.Clearenv()
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("PROVIDER", "invalid")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid provider")
	}
}

func TestLoadProxmoxProviderMissingFields(t *testing.T) {
	os.Clearenv()
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("PROVIDER", "proxmox")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing proxmox fields")
	}
}

func TestLoadProxmoxProviderValid(t *testing.T) {
	os.Clearenv()
	setEnvs(t, map[string]string{
		"JWT_SECRET":         "test-secret",
		"PROVIDER":           "proxmox",
		"PROXMOX_URL":        "https://pve.example.com:8006",
		"PROXMOX_TOKEN_ID":   "user@pam!tok",
		"PROXMOX_TOKEN_SECRET": "secret",
		"PROXMOX_NODE":       "pve1",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Provider != "proxmox" {
		t.Errorf("expected provider 'proxmox', got %q", cfg.Provider)
	}
	if cfg.ProxmoxNode != "pve1" {
		t.Errorf("expected node 'pve1', got %q", cfg.ProxmoxNode)
	}
}

func TestParseTemplateMap(t *testing.T) {
	cases := []struct {
		input string
		want  map[string]int
	}{
		{"", map[string]int{}},
		{"workstation=900", map[string]int{"workstation": 900}},
		{"ws=900,cp=901,worker=902", map[string]int{"ws": 900, "cp": 901, "worker": 902}},
		{" ws = 900 , cp = 901 ", map[string]int{"ws": 900, "cp": 901}},
		{"badformat", map[string]int{}},
		{"ws=notanumber", map[string]int{}},
	}

	for _, tc := range cases {
		got := parseTemplateMap(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("parseTemplateMap(%q): got %d entries, want %d", tc.input, len(got), len(tc.want))
			continue
		}
		for k, v := range tc.want {
			if got[k] != v {
				t.Errorf("parseTemplateMap(%q)[%s] = %d, want %d", tc.input, k, got[k], v)
			}
		}
	}
}

func TestGetIntEnv(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	if got := getIntEnv("TEST_INT", 0); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}

	if got := getIntEnv("TEST_INT_MISSING", 99); got != 99 {
		t.Errorf("expected 99, got %d", got)
	}

	t.Setenv("TEST_INT_BAD", "notanumber")
	if got := getIntEnv("TEST_INT_BAD", 7); got != 7 {
		t.Errorf("expected fallback 7, got %d", got)
	}
}
