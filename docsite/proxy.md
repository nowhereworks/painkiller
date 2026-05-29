# Documentation Proxy

Painkiller Shell uses a shared Squid proxy to restrict student workstation internet access to official documentation only. This prevents students from accessing search engines, AI tools, or other resources during tests.

## Architecture

```
Student Workstation
  ↓
  iptables (blocks direct HTTP/HTTPS)
  ↓
Squid Proxy (allowlist filtering)
  ↓
Official Documentation Sites
```

Student workstations are configured with:
- `http_proxy` and `https_proxy` environment variables pointing to the proxy
- `iptables` rules blocking direct outbound HTTP/HTTPS (ports 80/443) except to the proxy IP

## Deployment

### 1. Install Squid

Install Squid on a management network host:

**Ubuntu/Debian:**
```bash
sudo apt-get update
sudo apt-get install squid
```

**CentOS/RHEL:**
```bash
sudo yum install squid
```

### 2. Configure Squid

Copy the configuration files from the repository:

```bash
sudo cp infra/proxy/squid.conf /etc/squid/squid.conf
sudo cp infra/proxy/allowlist.txt /etc/squid/allowlist.txt
```

#### squid.conf

The main Squid configuration file:

```squid
# /etc/squid/squid.conf

# Listen on port 3128
http_port 3128

# Define ACL for allowlisted domains
acl allowlist dstdomain "/etc/squid/allowlist.txt"

# Define ACL for SSL ports
acl SSL_ports port 443
acl Safe_ports port 80
acl Safe_ports port 443
acl CONNECT method CONNECT

# Deny requests to non-safe ports
http_access deny !Safe_ports

# Deny CONNECT to non-SSL ports
http_access deny CONNECT !SSL_ports

# Allow access to allowlisted domains only
http_access allow allowlist

# Deny all other requests
http_access deny all

# Logging
access_log /var/log/squid/access.log squid
cache_log /var/log/squid/cache.log

# Disable caching (we only want filtering)
cache deny all

# Hide proxy headers for privacy
via off
forwarded_for off
```

#### allowlist.txt

The allowlist contains one domain per line. Use a leading dot for subdomains:

```
# Kubernetes documentation
kubernetes.io
.kubernetes.io

# Helm documentation
helm.sh
.helm.sh

# Docker documentation
docker.com
.docker.com
docs.docker.com

# Official cloud provider docs (if needed)
# aws.amazon.com
# cloud.google.com
# docs.microsoft.com
```

**Allowlist Policy:**

**Allowed:**
- Official Kubernetes documentation
- Official Helm documentation
- Official Docker documentation
- Package mirrors required by provisioning (if needed)

**Blocked:**
- General internet
- Search engines (Google, Bing, DuckDuckGo)
- AI tools (ChatGPT, Claude, GitHub Copilot)
- Paste sites (Pastebin, GitHub Gist)
- Other student environments
- Platform internals (Proxmox, database, backend)

### 3. Start Squid

Enable and start the Squid service:

```bash
sudo systemctl enable squid
sudo systemctl start squid
```

Verify Squid is running:

```bash
sudo systemctl status squid
```

### 4. Configure Firewall

Allow traffic to the proxy port:

```bash
sudo ufw allow 3128/tcp
# or
sudo iptables -A INPUT -p tcp --dport 3128 -j ACCEPT
```

### 5. Configure Workstation Template

The workstation VM template must include the iptables enforcement script.

Copy the script to your workstation template:

```bash
sudo cp infra/workstation/iptables.sh /opt/painkiller/iptables.sh
sudo chmod +x /opt/painkiller/iptables.sh
```

Edit the script to set your proxy IP:

```bash
sudo nano /opt/painkiller/iptables.sh
```

Update these variables:

```bash
PROXY_IP=10.0.0.100  # Your Squid proxy IP
PROXY_PORT=3128
```

The script will be executed during workstation provisioning to:
- Set `http_proxy` and `https_proxy` environment variables in `/etc/environment`
- Configure iptables to block direct outbound HTTP/HTTPS
- Allow traffic only to the proxy IP
- Save rules for persistence

### 6. Verify Configuration

From a student workstation, test the proxy:

**Test allowed access:**
```bash
curl -I https://kubernetes.io
# Should succeed (HTTP 200 or redirect)
```

**Test blocked access:**
```bash
curl -I https://google.com
# Should timeout or fail (connection refused)
```

**Test direct access (should be blocked):**
```bash
# Temporarily unset proxy
unset http_proxy https_proxy
curl -I https://kubernetes.io
# Should timeout or fail (iptables blocking)
```

## Managing the Allowlist

### Adding Domains

Edit the allowlist file:

```bash
sudo nano /etc/squid/allowlist.txt
```

Add new domains (one per line):

```
# Add Helm documentation
helm.sh
.helm.sh

# Add specific subdomain only
docs.example.com
```

Restart Squid to apply changes:

```bash
sudo systemctl restart squid
```

### Removing Domains

Remove lines from `/etc/squid/allowlist.txt` and restart Squid:

```bash
sudo systemctl restart squid
```

### Testing Allowlist Changes

Test a domain before adding to production:

```bash
# From the proxy server
squidclient -h localhost -p 3128 https://example.com
```

Check Squid logs for access decisions:

```bash
sudo tail -f /var/log/squid/access.log
```

Log format:
```
1717000000.000    123 10.100.0.5 TCP_MISS/200 1234 GET https://kubernetes.io/ - HIER_DIRECT/147.75.80.111 text/html
1717000001.000      0 10.100.0.5 TCP_DENIED/403 0 GET https://google.com/ - HIER_NONE/- text/html
```

- `TCP_MISS/200` - Request allowed and forwarded
- `TCP_DENIED/403` - Request blocked by ACL

## Monitoring

### Access Logs

View real-time access logs:

```bash
sudo tail -f /var/log/squid/access.log
```

Filter for specific student workstation:

```bash
sudo tail -f /var/log/squid/access.log | grep 10.100.0.5
```

### Cache Statistics

View Squid cache manager statistics:

```bash
sudo squidclient -h localhost mgr:info
```

### Resource Usage

Monitor Squid resource usage:

```bash
# CPU and memory
top -p $(pgrep squid)

# Disk usage (if caching enabled)
du -sh /var/spool/squid
```

## Troubleshooting

### Proxy Unreachable from Workstation

**Check network connectivity:**
```bash
ping <PROXY_IP>
telnet <PROXY_IP> 3128
```

**Check Squid is running:**
```bash
sudo systemctl status squid
```

**Check firewall rules:**
```bash
sudo ufw status
# or
sudo iptables -L INPUT -n | grep 3128
```

### Allowed Site Blocked

**Verify domain is in allowlist:**
```bash
grep -i "kubernetes.io" /etc/squid/allowlist.txt
```

**Check Squid logs:**
```bash
sudo tail -f /var/log/squid/access.log
```

**Test with squidclient:**
```bash
squidclient -h localhost -p 3128 https://kubernetes.io
```

**Common issues:**
- Domain not in allowlist
- Subdomain not covered (use `.example.com` for all subdomains)
- Squid not restarted after allowlist change

### Direct Internet Access Possible

**Verify iptables rules are active:**
```bash
sudo iptables -L OUTPUT -n -v
```

Expected output:
```
Chain OUTPUT (policy ACCEPT 0 packets, 0 bytes)
 pkts bytes target     prot opt in     out     source               destination
  100  8000 ACCEPT     tcp  --  *      *       0.0.0.0/0            10.0.0.100         tcp dpt:80
  200 16000 ACCEPT     tcp  --  *      *       0.0.0.0/0            10.0.0.100         tcp dpt:443
   50  4000 DROP       tcp  --  *      *       0.0.0.0/0            0.0.0.0/0          tcp dpt:80
   75  6000 DROP       tcp  --  *      *       0.0.0.0/0            0.0.0.0/0          tcp dpt:443
```

**Reapply rules:**
```bash
sudo bash /opt/painkiller/iptables.sh
```

**Check rules persist after reboot:**
```bash
sudo cat /etc/iptables/rules.v4
```

### Squid Won't Start

**Check configuration syntax:**
```bash
sudo squid -k parse
```

**Check Squid logs:**
```bash
sudo tail -n 50 /var/log/squid/cache.log
```

**Common issues:**
- Syntax error in `squid.conf`
- Allowlist file not readable
- Port 3128 already in use

### Performance Issues

**Check connection count:**
```bash
netstat -an | grep :3128 | wc -l
```

**Increase Squid limits** (edit `/etc/squid/squid.conf`):
```squid
# Increase max connections
max_filedescriptors 65535

# Increase client connections
client_persistent_connections on
```

Restart Squid:
```bash
sudo systemctl restart squid
```

## High Availability

For production deployments, consider high availability:

### Multiple Proxy Instances

Deploy multiple Squid instances and use a load balancer:

```
Student Workstation
  ↓
Load Balancer (HAProxy, nginx)
  ↓
Squid Proxy 1, 2, 3
```

Update workstation template to use load balancer IP instead of single proxy IP.

### Proxy Failover

Configure multiple proxies in workstation iptables:

```bash
# Allow traffic to multiple proxy IPs
iptables -A OUTPUT -p tcp -d 10.0.0.100 --dport 3128 -j ACCEPT
iptables -A OUTPUT -p tcp -d 10.0.0.101 --dport 3128 -j ACCEPT
iptables -A OUTPUT -p tcp -d 10.0.0.102 --dport 3128 -j ACCEPT
```

Set multiple proxies in environment variables:
```bash
http_proxy=http://10.0.0.100:3128,http://10.0.0.101:3128
```

## Security Considerations

### Proxy Authentication

For additional security, require proxy authentication:

```squid
# Add to squid.conf
auth_param basic program /usr/lib/squid/basic_ncsa_auth /etc/squid/passwd
auth_param basic realm Proxy Auth
acl authenticated proxy_auth REQUIRED
http_access allow authenticated allowlist
```

Create password file:
```bash
sudo htpasswd -c /etc/squid/passwd student
```

Update workstation template to include credentials in proxy URL:
```bash
http_proxy=http://student:password@10.0.0.100:3128
```

### HTTPS Inspection

By default, Squid only filters HTTPS by domain (CONNECT tunneling). To inspect HTTPS content:

1. Generate CA certificate
2. Configure Squid for SSL bumping
3. Install CA cert on workstations

**Warning:** HTTPS inspection is complex and may break some sites. For MVP, domain filtering is sufficient.

### Logging and Privacy

Squid logs all requests. Consider:
- Log rotation to manage disk space
- Log retention policy
- Privacy compliance (GDPR, etc.)

Configure log rotation:
```bash
sudo nano /etc/logrotate.d/squid
```

Example:
```
/var/log/squid/access.log {
    daily
    rotate 30
    compress
    delaycompress
    notifempty
    missingok
    postrotate
        /usr/sbin/squid -k rotate
    endscript
}
```

## Best Practices

1. **Dedicated host** - Run Squid on a dedicated host or VM
2. **Monitoring** - Monitor proxy logs and resource usage
3. **Backups** - Back up configuration files
4. **Testing** - Test allowlist changes in staging first
5. **Documentation** - Document allowlist decisions and rationale
6. **Regular reviews** - Review allowlist periodically
7. **Alerting** - Set up alerts for proxy failures
8. **Capacity planning** - Monitor usage and scale as needed
9. **Security updates** - Keep Squid updated with security patches
10. **Disaster recovery** - Document recovery procedures
