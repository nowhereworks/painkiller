# Documentation Proxy

Painkiller Shell uses a shared external Squid proxy to restrict student workstation internet access to official documentation only.

Painkiller Shell does not own Squid allowlist policy. The application only configures student workstations with the proxy address and direct-egress blocking; Squid ACLs, allowlists, authentication, caching, and filtering rules are configured in Squid or the operator's proxy deployment.

## Architecture

```
Student Workstation -> Squid Proxy -> Allowlisted Documentation Sites
```

Student workstations are configured with:
- `http_proxy` and `https_proxy` environment variables pointing to the proxy
- `iptables` rules blocking direct outbound HTTP/HTTPS (ports 80/443) except to the proxy IP

Allowed domains are configured on the Squid host, not through Painkiller environment variables.

## Deployment

### 1. Deploy Squid Proxy

Install Squid on a management network host:

```bash
apt-get install squid
```

Copy configuration:

```bash
cp infra/proxy/squid.conf /etc/squid/squid.conf
cp infra/proxy/allowlist.txt /etc/squid/allowlist.txt
systemctl restart squid
```

### 2. Update Allowlist

Edit `/etc/squid/allowlist.txt` to add or remove allowed domains:

```
kubernetes.io
.kubernetes.io
helm.sh
.helm.sh
```

Restart Squid after changes:

```bash
systemctl restart squid
```

### 3. Configure Workstation Template

The workstation VM template must include the iptables script. Update `PROXY_IP` in the script to match your proxy's IP address:

```bash
export PROXY_IP=10.0.0.100
export PROXY_PORT=3128
bash infra/workstation/iptables.sh
```

This script:
- Sets proxy environment variables in `/etc/environment`
- Configures iptables to block direct outbound HTTP/HTTPS
- Saves rules to `/etc/iptables/rules.v4` for persistence

### 4. Verify Configuration

From a student workstation, test allowed access:

```bash
curl -I https://kubernetes.io
```

Test blocked access:

```bash
curl -I https://google.com
# Should timeout or fail
```

## Allowlist Policy

**Allowed:**
- Official Kubernetes documentation
- Official Helm documentation
- Official Docker documentation
- Package mirrors required by provisioning

**Blocked:**
- General internet
- Search engines
- AI tools
- Paste sites
- Other student environments
- Platform internals (Proxmox, database, backend)

## Troubleshooting

### Proxy unreachable from workstation

Check network connectivity:

```bash
ping <PROXY_IP>
telnet <PROXY_IP> 3128
```

### Allowed site blocked

Verify domain is in allowlist:

```bash
grep -i "kubernetes.io" /etc/squid/allowlist.txt
```

Check Squid logs:

```bash
tail -f /var/log/squid/access.log
```

### Direct internet access possible

Verify iptables rules are active:

```bash
iptables -L OUTPUT -n -v
```

Reapply rules:

```bash
bash /path/to/iptables.sh
```
