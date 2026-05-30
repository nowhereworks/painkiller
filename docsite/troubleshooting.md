# Troubleshooting

This guide covers common issues and their resolutions when operating Painkiller Shell.

## Server Issues

### Server Won't Start

**Symptom:** Server exits immediately after starting

**Check logs:**
```bash
journalctl -u painkiller -n 50
# or
./server 2>&1 | head -n 50
```

**Common causes:**

1. **Missing required configuration**
   ```
   Error: DATABASE_URL is required
   ```
   **Solution:** Set `DATABASE_URL` in `.env`

2. **Database connection failed**
   ```
   Error: failed to connect to database: dial tcp 127.0.0.1:5432: connect: connection refused
   ```
   **Solution:** Start PostgreSQL or check `DATABASE_URL`

3. **Port already in use**
   ```
   Error: listen tcp :8080: bind: address already in use
   ```
   **Solution:** Change `HTTP_ADDR` or stop the conflicting service

4. **Invalid JWT secret**
   ```
   Error: JWT_SECRET must be at least 32 characters
   ```
   **Solution:** Set a strong `JWT_SECRET` (32+ characters)

5. **Job queue startup failed**
   ```
   Error: failed to start job queue: at least one Worker must be added to the Workers bundle
   ```
   **Solution:** Use `make run-dev` for local compose testing so the image is rebuilt and migrations run before the server starts. If running manually, run `go run ./cmd/migrate -direction up` before starting `go run ./cmd/server`.

### High Memory Usage

**Symptom:** Server memory usage grows over time

**Check:**
```bash
# Check memory usage
ps aux | grep server

# Check for memory leaks
go tool pprof http://localhost:8080/debug/pprof/heap
```

**Common causes:**

1. **Connection pool exhaustion**
   - **Solution:** Reduce `MaxOpenConns` in `internal/store/store.go`

2. **Job queue backlog**
   - **Solution:** Increase worker concurrency or add more workers

3. **Memory leak in code**
   - **Solution:** Profile with pprof and fix the leak

### High CPU Usage

**Symptom:** Server CPU usage is consistently high

**Check:**
```bash
# Check CPU usage
top -p $(pgrep server)

# Profile CPU
go tool pprof http://localhost:8080/debug/pprof/profile
```

**Common causes:**

1. **Too many concurrent requests**
   - **Solution:** Add rate limiting or scale horizontally

2. **Inefficient database queries**
   - **Solution:** Add indexes, optimize queries

3. **Tight loops in code**
   - **Solution:** Profile and fix hot paths

## Database Issues

### Connection Refused

**Symptom:** `connection refused` or `no such host`

**Check:**
```bash
# Verify PostgreSQL is running
systemctl status postgresql

# Test connection
psql "postgres://localhost:5432/painkiller?sslmode=disable" -c "SELECT 1"
```

**Solutions:**

1. **Start PostgreSQL:**
   ```bash
   sudo systemctl start postgresql
   ```

2. **Check `DATABASE_URL`:**
   ```bash
   echo $DATABASE_URL
   ```

3. **Verify database exists:**
   ```bash
   psql -l | grep painkiller
   ```

4. **Check PostgreSQL logs:**
   ```bash
   sudo journalctl -u postgresql -n 50
   ```

### Migrations Fail

**Symptom:** `make migrate-up` fails with an error

**Check:**
```bash
make migrate-up 2>&1
```

**Common causes:**

1. **Syntax error in migration:**
   ```
   Error: migration failed: syntax error at or near "CREAT"
   ```
   **Solution:** Fix the SQL syntax in the migration file

2. **Duplicate table:**
   ```
   Error: migration failed: relation "users" already exists
   ```
   **Solution:** Roll back and fix the migration:
   ```bash
   make migrate-down
   # Fix migration
   make migrate-up
   ```

3. **Foreign key constraint:**
   ```
   Error: migration failed: there is no unique constraint matching given keys
   ```
   **Solution:** Ensure referenced columns have unique constraints

### Slow Queries

**Symptom:** API responses are slow

**Check:**
```sql
-- Enable query logging
ALTER SYSTEM SET log_min_duration_statement = 1000;
SELECT pg_reload_conf();

-- Check slow queries
tail -f /var/log/postgresql/postgresql-14-main.log
```

**Solutions:**

1. **Add indexes:**
   ```sql
   CREATE INDEX idx_attempts_status ON attempts(status);
   CREATE INDEX idx_environments_attempt_id ON environments(attempt_id);
   ```

2. **Analyze query plans:**
   ```sql
   EXPLAIN ANALYZE SELECT * FROM attempts WHERE status = 'running';
   ```

3. **Vacuum and analyze:**
   ```sql
   VACUUM ANALYZE attempts;
   VACUUM ANALYZE environments;
   ```

### Database Lock Issues

**Symptom:** Queries hang or timeout

**Check:**
```sql
-- Find blocking queries
SELECT 
  blocked_locks.pid AS blocked_pid,
  blocked_activity.usename AS blocked_user,
  blocking_locks.pid AS blocking_pid,
  blocking_activity.usename AS blocking_user
FROM pg_catalog.pg_locks blocked_locks
JOIN pg_catalog.pg_stat_activity blocked_activity ON blocked_activity.pid = blocked_locks.pid
JOIN pg_catalog.pg_locks blocking_locks ON blocking_locks.locktype = blocked_locks.locktype
JOIN pg_catalog.pg_stat_activity blocking_activity ON blocking_activity.pid = blocking_locks.pid
WHERE NOT blocked_locks.granted;
```

**Solution:**
```sql
-- Kill blocking query
SELECT pg_terminate_backend(<blocking_pid>);
```

## Proxmox Issues

### API Connection Failed

**Symptom:** `connection refused` or `certificate signed by unknown authority`

**Check:**
```bash
# Test API connection
curl -k https://proxmox.example.com:8006/api2/json/version
```

**Solutions:**

1. **Verify Proxmox URL:**
   ```bash
   echo $PROXMOX_URL
   ```

2. **Check Proxmox is running:**
   ```bash
   ssh root@proxmox.example.com "systemctl status pveproxy"
   ```

3. **Accept self-signed certificate:**
   - The application uses `-k` flag to skip certificate verification
   - For production, install proper SSL certificates

### Permission Denied

**Symptom:** `403 Forbidden` or `permission denied`

**Check:**
```bash
# Test API token
curl -k -H "Authorization: PVEAPIToken=$PROXMOX_TOKEN_ID=$PROXMOX_TOKEN_SECRET" \
  https://proxmox.example.com:8006/api2/json/nodes
```

**Solutions:**

1. **Verify token ID format:**
   ```bash
   echo $PROXMOX_TOKEN_ID
   # Should be: user@realm!tokenname
   ```

2. **Check token permissions:**
   ```bash
   ssh root@proxmox.example.com "pveum user token list painkiller@pam"
   ```

3. **Grant permissions:**
   ```bash
   ssh root@proxmox.example.com "pveum aclmod / -user painkiller@pam -role PVEVMAdmin"
   ```

   If the API token has privilege separation enabled, grant permissions directly to the token. This fixes errors like `Permission check failed (/vms/9010, VM.Clone)`:

   ```bash
   ssh root@proxmox.example.com "pveum aclmod /vms/9010 -token 'painkiller@pam!api' -role PVEVMAdmin"
   ssh root@proxmox.example.com "pveum aclmod /storage/local-lvm -token 'painkiller@pam!api' -role PVEDatastoreUser"
   ```

   Replace `9010` with the workstation template VMID from `PROXMOX_TEMPLATES`, and replace `local-lvm` with `PROXMOX_STORAGE_POOL`.

### VM Creation Failed

**Symptom:** `provision_environment` job fails

**Check logs:**
```bash
journalctl -u painkiller | grep "provision_environment"
```

**Common causes:**

1. **Template not found:**
   ```
   Error: template 9001 not found
   ```
   **Solution:** Verify template exists:
   ```bash
   ssh root@proxmox.example.com "qm list | grep 9001"
   ```

2. **Insufficient resources:**
   ```
   Error: not enough memory
   ```
   **Solution:** Free up resources or add more RAM

3. **Storage full:**
   ```
   Error: storage 'local-lvm' is full
   ```
   **Solution:** Clean up old VMs or expand storage

4. **Missing Proxmox clone VMID:**
   ```
   errors":{"newid":"property is missing and it is not optional"}
   ```
   **Solution:** Rebuild and redeploy the Painkiller server. Current versions call Proxmox `/cluster/nextid`, send that value as `newid` on clone requests, and retry with a fresh VMID if another process uses the same ID first.

### VM Won't Start

**Symptom:** VM created but stuck in `stopped` state

**Check:**
```bash
ssh root@proxmox.example.com "qm status <vmid>"
ssh root@proxmox.example.com "qm start <vmid>"
```

**Common causes:**

1. **Cloud-init error:**
   ```bash
   ssh root@proxmox.example.com "qm guest cmd <vmid> exec 'cat /var/log/cloud-init-output.log'"
   ```

2. **Network configuration error:**
   ```bash
   ssh root@proxmox.example.com "qm config <vmid> | grep net"
   ```

3. **Resource conflict:**
   ```bash
   ssh root@proxmox.example.com "qm list" | grep <vmid>
   ```

## Terminal Issues

### WebSocket Connection Failed

**Symptom:** Terminal in browser shows "Connection failed"

**Check browser console:**
```
WebSocket connection to 'ws://localhost:8080/api/v1/terminal/token' failed
```

**Solutions:**

1. **Verify token is valid:**
   ```bash
   curl -H "Authorization: Bearer $TOKEN" \
     http://localhost:8080/api/v1/attempts/uuid
   ```

2. **Check attempt status:**
   - Must be `environment_ready` or `running`
   - Cannot connect to `provisioning` or `scored` attempts

3. **Verify workstation is accessible:**
   ```bash
   ping <workstation_ip>
   ssh -i /path/to/key ubuntu@<workstation_ip>
   ```

### SSH Connection Failed

**Symptom:** Terminal connects but shows blank screen or error

**Check logs:**
```bash
journalctl -u painkiller | grep "terminal"
```

**Common causes:**

1. **SSH key not found:**
   ```
   Error: failed to read SSH key: no such file
   ```
   **Solution:** Check environment metadata has SSH key

2. **SSH connection refused:**
   ```
   Error: dial tcp 10.100.0.5:22: connect: connection refused
   ```
   **Solution:** Verify SSH is running on workstation:
   ```bash
   ssh ubuntu@<workstation_ip> "systemctl status ssh"
   ```

3. **Authentication failed:**
   ```
   Error: ssh: handshake failed: ssh: unable to authenticate
   ```
   **Solution:** Verify SSH public key was injected into workstation

### Terminal Disconnects Frequently

**Symptom:** Terminal drops connection after a few minutes

**Check:**
```bash
# Check for network issues
ping <workstation_ip>

# Check server logs
journalctl -u painkiller | grep "websocket"
```

**Solutions:**

1. **Increase WebSocket timeout:**
   Edit `internal/terminal/gateway.go`:
   ```go
   conn.SetReadDeadline(time.Now().Add(60 * time.Second))
   ```

2. **Enable keepalive:**
   ```go
   conn.SetPongHandler(func(string) error {
     conn.SetReadDeadline(time.Now().Add(60 * time.Second))
     return nil
   })
   ```

3. **Check network stability:**
   - Verify no firewall dropping idle connections
   - Check load balancer WebSocket settings

## Grading Issues

### Grading Job Fails

**Symptom:** Attempt stuck in `grading` status

**Check logs:**
```bash
journalctl -u painkiller | grep "grade_attempt"
```

**Common causes:**

1. **SSH connection failed:**
   ```
   Error: failed to connect to workstation: dial tcp 10.100.0.5:22: i/o timeout
   ```
   **Solution:** Verify workstation is accessible

2. **Check script error:**
   ```
   Error: check script exited with code 127
   ```
   **Solution:** Verify script exists and is executable

3. **kubectl context error:**
   ```
   Error: the server doesn't have a resource type "networkpolicy"
   ```
   **Solution:** Verify kubeconfig and cluster access

### Incorrect Scores

**Symptom:** Student reports wrong score

**Check:**
```sql
SELECT 
  c.id,
  c.task_id,
  cr.passed,
  cr.points_awarded,
  cr.points_possible,
  cr.stdout,
  cr.stderr
FROM checks c
JOIN check_results cr ON c.id = cr.check_id
WHERE cr.attempt_id = 'uuid'
ORDER BY c.task_id;
```

**Solutions:**

1. **Review check logic:**
   - Verify check command is correct
   - Test check manually on workstation

2. **Check for race conditions:**
   - Ensure checks wait for resources to be ready
   - Add retry logic if needed

3. **Re-run grading:**
   ```bash
   curl -X POST -H "Authorization: Bearer $TOKEN" \
     http://localhost:8080/api/v1/admin/attempts/uuid/retry-grade
   ```

## Billing Issues

### Stripe Webhook Not Received

**Symptom:** Payment completes but no `PurchasedTest` created

**Check:**
```bash
# Check Stripe dashboard for webhook delivery
# https://dashboard.stripe.com/webhooks

# Check server logs
journalctl -u painkiller | grep "stripe"
```

**Solutions:**

1. **Verify webhook URL:**
   - Must be publicly accessible (not localhost)
   - Use ngrok for testing: `ngrok http 8080`

2. **Check webhook secret:**
   ```bash
   echo $STRIPE_WEBHOOK_SECRET
   ```

3. **Test webhook locally:**
   ```bash
   stripe listen --forward-to localhost:8080/api/v1/webhooks/stripe
   stripe trigger checkout.session.completed
   ```

### Checkout Session Fails

**Symptom:** `/api/v1/checkout` returns error

**Check logs:**
```bash
journalctl -u painkiller | grep "checkout"
```

**Common causes:**

1. **Invalid Stripe key:**
   ```
   Error: Invalid API Key provided
   ```
   **Solution:** Verify `STRIPE_SECRET_KEY`

2. **Product not found:**
   ```
   Error: No such price: 'price_123'
   ```
   **Solution:** Verify product has valid `stripe_price_id`

3. **Missing success/cancel URLs:**
   ```
   Error: You must provide success_url
   ```
   **Solution:** Set `STRIPE_SUCCESS_URL` and `STRIPE_CANCEL_URL`

## Proxy Issues

### Proxy Unreachable

**Symptom:** Student workstation cannot access documentation

**Check from workstation:**
```bash
curl -I https://kubernetes.io
# Should succeed

curl -x http://10.0.0.100:3128 https://kubernetes.io
# Should succeed
```

**Solutions:**

1. **Verify proxy is running:**
   ```bash
   ssh proxy-host "systemctl status squid"
   ```

2. **Check firewall:**
   ```bash
   ssh proxy-host "sudo ufw status"
   ```

3. **Test proxy directly:**
   ```bash
   squidclient -h localhost -p 3128 https://kubernetes.io
   ```

### Allowed Site Blocked

**Symptom:** Student cannot access a documentation site

**Check:**
```bash
# Check allowlist
ssh proxy-host "grep kubernetes.io /etc/squid/allowlist.txt"

# Check Squid logs
ssh proxy-host "sudo tail -f /var/log/squid/access.log"
```

**Solutions:**

1. **Add domain to allowlist:**
   ```bash
   ssh proxy-host "echo 'example.com' | sudo tee -a /etc/squid/allowlist.txt"
   ssh proxy-host "sudo systemctl restart squid"
   ```

2. **Check for subdomain issues:**
   - Use `.example.com` to allow all subdomains
   - Restart Squid after changes

### Direct Internet Access

**Symptom:** Student can bypass proxy and access internet directly

**Check from workstation:**
```bash
# Unset proxy
unset http_proxy https_proxy

# Try direct access
curl -I https://google.com
# Should fail (timeout or connection refused)
```

**Solutions:**

1. **Verify iptables rules:**
   ```bash
   sudo iptables -L OUTPUT -n -v
   ```

2. **Reapply iptables:**
   ```bash
   sudo bash /opt/painkiller/iptables.sh
   ```

3. **Check for rule persistence:**
   ```bash
   sudo cat /etc/iptables/rules.v4
   ```

## Performance Issues

### Slow Provisioning

**Symptom:** Environment provisioning takes > 10 minutes

**Check:**
```bash
# Check job duration
SELECT 
  id,
  created_at,
  updated_at,
  EXTRACT(EPOCH FROM (updated_at - created_at)) as duration_seconds
FROM jobs
WHERE queue = 'provision_environment'
ORDER BY created_at DESC
LIMIT 10;
```

**Solutions:**

1. **Optimize VM templates:**
   - Pre-install packages
   - Use smaller disk images
   - Enable compression

2. **Parallelize provisioning:**
   - Clone VMs in parallel
   - Run Ansible playbooks concurrently

3. **Increase resources:**
   - Add more Proxmox nodes
   - Use faster storage (SSD/NVMe)

### High Job Queue Latency

**Symptom:** Jobs wait > 1 minute before processing

**Check:**
```sql
SELECT 
  queue,
  COUNT(*) as pending_count,
  AVG(EXTRACT(EPOCH FROM (NOW() - created_at))) as avg_wait_seconds
FROM jobs
WHERE status = 'pending'
GROUP BY queue;
```

**Solutions:**

1. **Increase worker concurrency:**
   Edit `internal/jobs/worker.go`:
   ```go
   MaxWorkers: 20  // Increase from default
   ```

2. **Add more workers:**
   - Run multiple worker processes
   - Distribute across multiple hosts

3. **Optimize job handlers:**
   - Profile slow jobs
   - Add caching where appropriate

## Debugging

### Enable Debug Logging

Set log level to debug:

```bash
LOG_LEVEL=debug ./server
```

Debug logs include:
- Request/response details
- Database queries
- Job execution details
- SSH connection attempts

### Database Query Logging

Enable query logging in PostgreSQL:

```sql
ALTER SYSTEM SET log_statement = 'all';
SELECT pg_reload_conf();
```

View logs:
```bash
sudo tail -f /var/log/postgresql/postgresql-14-main.log
```

### Profiling

Enable pprof endpoints:

```go
import _ "net/http/pprof"

// In main.go
go func() {
  log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

Profile CPU:
```bash
go tool pprof http://localhost:6060/debug/pprof/profile
```

Profile memory:
```bash
go tool pprof http://localhost:6060/debug/pprof/heap
```

### Request Tracing

Add request ID middleware:

```go
func RequestIDMiddleware(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    requestID := uuid.New().String()
    r = r.WithContext(context.WithValue(r.Context(), "requestID", requestID))
    w.Header().Set("X-Request-ID", requestID)
    next.ServeHTTP(w, r)
  })
}
```

Trace requests through logs using the request ID.

## Getting Help

If you encounter an issue not covered here:

1. **Check logs:**
   ```bash
   journalctl -u painkiller -n 100
   ```

2. **Search issues:**
   - Check GitHub issues for similar problems
   - Review architecture and implementation docs

3. **Collect information:**
   - Server version
   - Configuration (redact secrets)
   - Relevant logs
   - Steps to reproduce

4. **Open an issue:**
   - Provide detailed description
   - Include logs and configuration
   - Describe expected vs actual behavior
