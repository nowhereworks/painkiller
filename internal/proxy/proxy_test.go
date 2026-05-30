package proxy

import (
	"strings"
	"testing"
)

func TestGenerateIPTablesScript(t *testing.T) {
	cfg := Config{
		Addr: "10.0.0.10:3128",
	}
	clusterCIDRs := []string{"10.244.0.0/16", "192.168.100.0/24"}

	script := GenerateIPTablesScript(cfg, clusterCIDRs)

	if !strings.Contains(script, "#!/bin/bash") {
		t.Error("expected shebang line")
	}
	if !strings.Contains(script, "10.0.0.10") {
		t.Error("expected proxy IP in script")
	}
	if !strings.Contains(script, "3128") {
		t.Error("expected proxy port in script")
	}
	if !strings.Contains(script, "10.244.0.0/16") {
		t.Error("expected cluster CIDR in script")
	}
	if !strings.Contains(script, "192.168.100.0/24") {
		t.Error("expected cluster CIDR in script")
	}
	if !strings.Contains(script, "--dport 80 -j DROP") {
		t.Error("expected HTTP block rule")
	}
	if !strings.Contains(script, "--dport 443 -j DROP") {
		t.Error("expected HTTPS block rule")
	}
	if !strings.Contains(script, "--dport 53 -j ACCEPT") {
		t.Error("expected DNS allow rule")
	}
}

func TestGenerateIPTablesScriptDefaultPort(t *testing.T) {
	cfg := Config{
		Addr: "10.0.0.10",
	}

	script := GenerateIPTablesScript(cfg, nil)

	if !strings.Contains(script, "3128") {
		t.Error("expected default port 3128")
	}
}

func TestParseAddr(t *testing.T) {
	cases := []struct {
		input    string
		wantHost string
		wantPort string
	}{
		{"10.0.0.10:3128", "10.0.0.10", "3128"},
		{"proxy.example.com:8080", "proxy.example.com", "8080"},
		{"10.0.0.10", "10.0.0.10", "3128"},
	}

	for _, tc := range cases {
		host, port := parseAddr(tc.input)
		if host != tc.wantHost {
			t.Errorf("parseAddr(%q) host = %q, want %q", tc.input, host, tc.wantHost)
		}
		if port != tc.wantPort {
			t.Errorf("parseAddr(%q) port = %q, want %q", tc.input, port, tc.wantPort)
		}
	}
}
