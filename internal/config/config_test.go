package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setEnvs(t *testing.T, envs map[string]string) {
	t.Helper()
	for k, v := range envs {
		t.Setenv(k, v)
	}
}

func writeProfilesFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "proxmox-profiles.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write profiles file: %v", err)
	}
	return path
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
		"JWT_SECRET":           "test-secret",
		"PROVIDER":             "proxmox",
		"PROXMOX_URL":          "https://pve.example.com:8006",
		"PROXMOX_TOKEN_ID":     "user@pam!tok",
		"PROXMOX_TOKEN_SECRET": "secret",
		"PROXMOX_NODE":         "pve1",
		"PROXMOX_PROFILES_FILE": writeProfilesFile(t, `profiles:
  workstation:
    template_vmid: 900
    config:
      citype: nocloud
      ipconfig0: ip=dhcp
      sshkeys: "{{ ssh_public_key }}"
`),
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
	if cfg.ProxmoxProfiles["workstation"].TemplateVMID != 900 {
		t.Errorf("expected workstation template VMID 900, got %d", cfg.ProxmoxProfiles["workstation"].TemplateVMID)
	}
}

func TestLoadProxmoxProviderRequiresProfilesFile(t *testing.T) {
	os.Clearenv()
	setEnvs(t, map[string]string{
		"JWT_SECRET":           "test-secret",
		"PROVIDER":             "proxmox",
		"PROXMOX_URL":          "https://pve.example.com:8006",
		"PROXMOX_TOKEN_ID":     "user@pam!tok",
		"PROXMOX_TOKEN_SECRET": "secret",
		"PROXMOX_NODE":         "pve1",
	})

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "PROXMOX_PROFILES_FILE") {
		t.Fatalf("expected PROXMOX_PROFILES_FILE error, got %v", err)
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
