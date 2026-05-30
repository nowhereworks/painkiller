package proxy

import (
	"fmt"
	"strings"
)

func GenerateSquidConf(cfg Config) string {
	var b strings.Builder

	b.WriteString("# Painkiller Shell - Restricted Documentation Proxy\n\n")

	b.WriteString("http_port 3128\n\n")

	b.WriteString("# ACL definitions\n")
	b.WriteString("acl localnet src 10.0.0.0/8\n")
	b.WriteString("acl localnet src 172.16.0.0/12\n")
	b.WriteString("acl localnet src 192.168.0.0/16\n")
	b.WriteString("acl SSL_ports port 443\n")
	b.WriteString("acl Safe_ports port 80\n")
	b.WriteString("acl Safe_ports port 443\n")
	b.WriteString("acl CONNECT method CONNECT\n\n")

	b.WriteString("# Deny requests to non-safe ports\n")
	b.WriteString("http_access deny !Safe_ports\n\n")

	b.WriteString("# Deny CONNECT to non-SSL ports\n")
	b.WriteString("http_access deny CONNECT !SSL_ports\n\n")

	b.WriteString("# Allowlist ACL for documentation domains\n")
	b.WriteString("acl allowed_docs dstdomain")
	for _, domain := range cfg.AllowedDomains {
		b.WriteString(fmt.Sprintf(" .%s", domain))
	}
	b.WriteString("\n")
	b.WriteString("acl allowed_docs_exact dstdomain")
	for _, domain := range cfg.AllowedDomains {
		b.WriteString(fmt.Sprintf(" %s", domain))
	}
	b.WriteString("\n\n")

	b.WriteString("# Allow access to allowlisted documentation\n")
	b.WriteString("http_access allow localnet allowed_docs\n")
	b.WriteString("http_access allow localnet allowed_docs_exact\n\n")

	b.WriteString("# Deny everything else\n")
	b.WriteString("http_access deny all\n\n")

	b.WriteString("# Logging\n")
	b.WriteString("access_log /var/log/squid/access.log squid\n")
	b.WriteString("cache_log /var/log/squid/cache.log\n\n")

	b.WriteString("# Performance\n")
	b.WriteString("maximum_object_size 50 MB\n")
	b.WriteString("cache_dir ufs /var/spool/squid 1000 16 256\n")
	b.WriteString("coredump_dir /var/spool/squid\n\n")

	b.WriteString("# Timeouts\n")
	b.WriteString("connect_timeout 30 seconds\n")
	b.WriteString("read_timeout 60 seconds\n")
	b.WriteString("request_timeout 60 seconds\n\n")

	b.WriteString("# Privacy\n")
	b.WriteString("via off\n")
	b.WriteString("forwarded_for delete\n")
	b.WriteString("request_header_access X-Forwarded-For deny all\n")

	return b.String()
}
