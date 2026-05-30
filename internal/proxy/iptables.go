package proxy

import (
	"fmt"
	"strings"
)

func GenerateIPTablesScript(cfg Config, clusterCIDRs []string) string {
	var b strings.Builder

	b.WriteString("#!/bin/bash\nset -euo pipefail\n\n")

	b.WriteString("# Flush existing rules for ports 80 and 443\n")
	b.WriteString("iptables -D OUTPUT -p tcp --dport 80 -j DROP 2>/dev/null || true\n")
	b.WriteString("iptables -D OUTPUT -p tcp --dport 443 -j DROP 2>/dev/null || true\n")
	b.WriteString("iptables -D OUTPUT -p tcp --dport 80 -j ACCEPT 2>/dev/null || true\n")
	b.WriteString("iptables -D OUTPUT -p tcp --dport 443 -j ACCEPT 2>/dev/null || true\n\n")

	b.WriteString("# Allow DNS\n")
	b.WriteString("iptables -A OUTPUT -p udp --dport 53 -j ACCEPT\n")
	b.WriteString("iptables -A OUTPUT -p tcp --dport 53 -j ACCEPT\n\n")

	b.WriteString("# Allow traffic to cluster CIDRs\n")
	for _, cidr := range clusterCIDRs {
		b.WriteString(fmt.Sprintf("iptables -A OUTPUT -d %s -j ACCEPT\n", cidr))
	}
	b.WriteString("\n")

	b.WriteString("# Allow traffic to proxy\n")
	host, port := parseAddr(cfg.Addr)
	b.WriteString(fmt.Sprintf("iptables -A OUTPUT -p tcp -d %s --dport %s -j ACCEPT\n\n", host, port))

	b.WriteString("# Allow loopback\n")
	b.WriteString("iptables -A OUTPUT -o lo -j ACCEPT\n\n")

	b.WriteString("# Allow established connections\n")
	b.WriteString("iptables -A OUTPUT -m state --state ESTABLISHED,RELATED -j ACCEPT\n\n")

	b.WriteString("# Block all other outbound HTTP/HTTPS\n")
	b.WriteString("iptables -A OUTPUT -p tcp --dport 80 -j DROP\n")
	b.WriteString("iptables -A OUTPUT -p tcp --dport 443 -j DROP\n\n")

	b.WriteString("# Persist rules\n")
	b.WriteString("if command -v iptables-save >/dev/null 2>&1; then\n")
	b.WriteString("  iptables-save > /etc/iptables/rules.v4 2>/dev/null || true\n")
	b.WriteString("fi\n")

	return b.String()
}

func parseAddr(addr string) (string, string) {
	parts := strings.SplitN(addr, ":", 2)
	if len(parts) != 2 {
		return addr, "3128"
	}
	return parts[0], parts[1]
}
