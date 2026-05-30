package proxmox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"painkiller-shell/internal/provider"
)

func writeCloneProfilesFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profiles.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write profiles file: %v", err)
	}
	return path
}

func TestLoadCloneProfiles(t *testing.T) {
	path := writeCloneProfilesFile(t, `profiles:
  workstation:
    template_vmid: 9010
    config:
      citype: nocloud
      ipconfig0: ip=dhcp
      sshkeys: "{{ ssh_public_key }}"
  kubeadm-control-plane:
    template_vmid: 9011
    config:
      name: "{{ hostname }}"
`)

	profiles, err := LoadCloneProfiles(path)
	if err != nil {
		t.Fatalf("LoadCloneProfiles failed: %v", err)
	}
	if profiles["workstation"].TemplateVMID != 9010 {
		t.Fatalf("workstation template_vmid = %d, want 9010", profiles["workstation"].TemplateVMID)
	}
	if profiles["workstation"].Config["sshkeys"] != "{{ ssh_public_key }}" {
		t.Fatalf("sshkeys = %q", profiles["workstation"].Config["sshkeys"])
	}
}

func TestLoadCloneProfilesValidation(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name: "empty profiles",
			content: `profiles: {}
`,
			wantErr: "at least one profile",
		},
		{
			name: "bad template VMID",
			content: `profiles:
  workstation:
    template_vmid: 0
`,
			wantErr: "template_vmid greater than 0",
		},
		{
			name: "cipublickey",
			content: `profiles:
  workstation:
    template_vmid: 9010
    config:
      cipublickey: key
`,
			wantErr: `use "sshkeys" instead`,
		},
		{
			name: "ciname",
			content: `profiles:
  workstation:
    template_vmid: 9010
    config:
      ciname: ws
`,
			wantErr: "VM hostname is set during clone",
		},
		{
			name: "bridge in ipconfig",
			content: `profiles:
  workstation:
    template_vmid: 9010
    config:
      ipconfig0: ip=dhcp,bridge=vmbr0
`,
			wantErr: "bridge belongs in net0/template NIC config",
		},
		{
			name: "unknown placeholder",
			content: `profiles:
  workstation:
    template_vmid: 9010
    config:
      sshkeys: "{{ key }}"
`,
			wantErr: "unsupported placeholder",
		},
		{
			name: "invalid clone_mode",
			content: `profiles:
  workstation:
    template_vmid: 9010
    clone_mode: snapshot
`,
			wantErr: "invalid clone_mode",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadCloneProfiles(writeCloneProfilesFile(t, tc.content))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestRenderProfileConfig(t *testing.T) {
	p := &ProxmoxProvider{}
	got := p.renderProfileConfig(CloneProfile{Config: map[string]string{
		"sshkeys": "{{ssh_public_key}}",
		"name":    "{{  hostname  }}",
	}}, provider.VMRequest{Hostname: "ws-1", SSHPublicKey: " ssh-rsa test\n"})

	if got["sshkeys"] != "ssh-rsa test" {
		t.Fatalf("sshkeys = %q, want trimmed key", got["sshkeys"])
	}
	if got["name"] != "ws-1" {
		t.Fatalf("name = %q, want ws-1", got["name"])
	}
}

func TestProfileForUnknownProfile(t *testing.T) {
	p := &ProxmoxProvider{config: Config{Profiles: map[string]CloneProfile{}}}
	_, err := p.profileFor("missing")
	if err == nil || !strings.Contains(err.Error(), "unknown proxmox clone profile: missing") {
		t.Fatalf("error = %v", err)
	}
}

func TestCloneModeDefault(t *testing.T) {
	path := writeCloneProfilesFile(t, `profiles:
  workstation:
    template_vmid: 9010
    config:
      citype: nocloud
`)

	profiles, err := LoadCloneProfiles(path)
	if err != nil {
		t.Fatalf("LoadCloneProfiles failed: %v", err)
	}
	if profiles["workstation"].CloneMode != "linked" {
		t.Fatalf("CloneMode = %q, want 'linked'", profiles["workstation"].CloneMode)
	}
}

func TestCloneModeExplicit(t *testing.T) {
	path := writeCloneProfilesFile(t, `profiles:
  workstation:
    template_vmid: 9010
    clone_mode: full
    config:
      citype: nocloud
`)

	profiles, err := LoadCloneProfiles(path)
	if err != nil {
		t.Fatalf("LoadCloneProfiles failed: %v", err)
	}
	if profiles["workstation"].CloneMode != "full" {
		t.Fatalf("CloneMode = %q, want 'full'", profiles["workstation"].CloneMode)
	}
}
