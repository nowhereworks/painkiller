package proxmox

import (
	"fmt"
	"strings"
)

type CloudInitConfig struct {
	Hostname     string
	SSHPublicKey string
	NetworkConfig string
	Metadata     map[string]string
}

func GenerateCloudInit(cfg CloudInitConfig) string {
	var b strings.Builder

	b.WriteString("#cloud-config\n")
	b.WriteString(fmt.Sprintf("hostname: %s\n", cfg.Hostname))
	b.WriteString("manage_etc_hosts: true\n")

	if cfg.SSHPublicKey != "" {
		b.WriteString("ssh_authorized_keys:\n")
		b.WriteString(fmt.Sprintf("  - %s\n", cfg.SSHPublicKey))
	}

	if cfg.NetworkConfig != "" {
		b.WriteString("\n")
		b.WriteString(cfg.NetworkConfig)
	}

	if len(cfg.Metadata) > 0 {
		b.WriteString("\nwrite_files:\n")
		b.WriteString("  - path: /etc/painkiller-metadata\n")
		b.WriteString("    content: |\n")
		for k, v := range cfg.Metadata {
			b.WriteString(fmt.Sprintf("      %s=%s\n", k, v))
		}
	}

	return b.String()
}
