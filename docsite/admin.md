# Admin Controls

Painkiller Shell provides administrative API endpoints for monitoring and managing the platform. Admin access is controlled by the `is_admin` flag on user accounts.

## Admin Authentication

### Creating Admin Users

Admin users are regular users with the `is_admin` flag set to `true`.

**Via database:**

```sql
UPDATE users SET is_admin = true WHERE email = 'admin@example.com';
```

**Via migration:**

Create a migration to seed admin users:

```sql
-- migrations/000003_seed_admin.up.sql
INSERT INTO users (id, email, password_hash, is_admin, created_at)
VALUES (
  '00000000-0000-0000-0000-000000000001',
  'admin@example.com',
  '$2a$10$...', -- bcrypt hash of admin password
  true,
  NOW()
);
```

### Admin Middleware

All admin endpoints require:
1. Valid JWT token (authentication)
2. `is_admin = true` on the user account (authorization)

Non-admin users receive `403 Forbidden` on admin endpoints.

## Admin API Endpoints

All admin endpoints are prefixed with `/api/v1/admin`.

### Catalog Management

The admin UI at `/admin/` provides a web interface for managing the test catalog. The following API endpoints power it.

#### List Tests

List all tests with product details.

**Endpoint:** `GET /api/v1/admin/tests`

**Example:**
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/admin/tests
```

**Response:**
```json
{
  "tests": [
    {
      "id": "uuid",
      "product_id": "uuid",
      "title": "CKA Simulator 1",
      "description": "Full CKA practice exam",
      "stripe_price_id": "price_...",
      "is_free": false,
      "duration_minutes": 120,
      "access_window_hours": 36,
      "attempts_allowed": 2
    }
  ]
}
```

#### Get Test

Get a single test by ID.

**Endpoint:** `GET /api/v1/admin/tests/:id`

#### Create Test

Create a new product and test in a single transaction.

**Endpoint:** `POST /api/v1/admin/tests`

**Request body:**
```json
{
  "title": "CKA Simulator 1",
  "description": "Full CKA practice exam",
  "stripe_price_id": "price_...",
  "is_free": false,
  "duration_minutes": 120,
  "access_window_hours": 36,
  "attempts_allowed": 2
}
```

**Notes:**
- `stripe_price_id` is required when `is_free` is `false`.
- `stripe_price_id` is ignored when `is_free` is `true`.

#### Update Test

Update product and test fields. All fields are optional; only provided fields are updated.

**Endpoint:** `PUT /api/v1/admin/tests/:id`

**Request body (partial):**
```json
{
  "title": "Updated title",
  "is_free": true,
  "duration_minutes": 180
}
```

#### Delete Test

Delete a test and its associated product. Fails if any purchases exist for the test.

**Endpoint:** `DELETE /api/v1/admin/tests/:id`

### Free Tests

Products can be flagged as free (`is_free = true`). Free tests:
- Appear in the public catalog with a "Free" badge.
- Allow students to acquire access via `POST /api/v1/billing/acquire-free` without going through Stripe checkout.
- Do not require a `stripe_price_id`.

**Acquire free test:**
```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -d '{"test_id": "uuid"}' \
  http://localhost:8080/api/v1/billing/acquire-free
```

### User Info

Authenticated users can retrieve their profile to check admin status.

**Endpoint:** `GET /api/v1/auth/me`

**Response:**
```json
{
  "id": "uuid",
  "email": "admin@example.com",
  "is_admin": true
}
```

### List Attempts

List all attempts with optional status filtering.

**Endpoint:** `GET /api/v1/admin/attempts`

**Query Parameters:**
- `status` (optional) - Filter by attempt status (e.g., `running`, `provision_failed`)

**Example:**
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/admin/attempts?status=running
```

**Response:**
```json
[
  {
    "id": "uuid",
    "purchased_test_id": "uuid",
    "user_id": "uuid",
    "status": "running",
    "started_at": "2026-05-29T10:00:00Z",
    "ended_at": null,
    "score": null
  }
]
```

### List Environments

List all environments with status.

**Endpoint:** `GET /api/v1/admin/environments`

**Query Parameters:**
- `status` (optional) - Filter by environment status

**Example:**
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/admin/environments
```

**Response:**
```json
[
  {
    "id": "uuid",
    "attempt_id": "uuid",
    "status": "ready",
    "workstation_ip": "10.100.0.5",
    "provider_metadata": {...},
    "created_at": "2026-05-29T10:00:00Z"
  }
]
```

### Retry Provisioning

Retry provisioning for a failed attempt.

**Endpoint:** `POST /api/v1/admin/attempts/:id/retry-provision`

**Example:**
```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/admin/attempts/uuid/retry-provision
```

**Use Cases:**
- Attempt failed due to transient Proxmox error
- Network issue during provisioning
- Ansible playbook timeout

**Behavior:**
- Resets attempt status to `attempt_requested`
- Enqueues new `provision_environment` job
- Previous environment is cleaned up automatically
- Attempt count is automatically restored when provisioning fails, so students don't lose attempts due to infrastructure issues

### Retry Grading

Retry grading for an attempt.

**Endpoint:** `POST /api/v1/admin/attempts/:id/retry-grade`

**Example:**
```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/admin/attempts/uuid/retry-grade
```

**Use Cases:**
- Grading job failed due to SSH connection issue
- Check script had transient error
- Manual intervention required

**Behavior:**
- Enqueues new `grade_attempt` job
- Previous grading results are overwritten

### Force Destroy Environment

Manually destroy an environment.

**Endpoint:** `POST /api/v1/admin/environments/:id/destroy`

**Example:**
```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/admin/environments/uuid/destroy
```

**Use Cases:**
- Environment stuck in `destroying` state
- Orphaned environment not cleaned up
- Manual cleanup required

**Behavior:**
- Enqueues `cleanup_environment` job
- Transitions environment to `destroying`
- Proxmox VMs are stopped and deleted

## Monitoring

### Dashboard Queries

Useful SQL queries for monitoring:

**Active attempts:**
```sql
SELECT 
  a.id,
  u.email,
  t.title,
  a.status,
  a.started_at,
  e.workstation_ip
FROM attempts a
JOIN purchased_tests pt ON a.purchased_test_id = pt.id
JOIN users u ON pt.user_id = u.id
JOIN tests t ON pt.test_id = t.id
LEFT JOIN environments e ON a.id = e.attempt_id
WHERE a.status IN ('running', 'environment_ready', 'terminal_opened')
ORDER BY a.started_at DESC;
```

**Failed provisioning:**
```sql
SELECT 
  a.id,
  u.email,
  t.title,
  a.status,
  a.started_at,
  e.status as env_status
FROM attempts a
JOIN purchased_tests pt ON a.purchased_test_id = pt.id
JOIN users u ON pt.user_id = u.id
JOIN tests t ON pt.test_id = t.id
LEFT JOIN environments e ON a.id = e.attempt_id
WHERE a.status = 'provision_failed'
ORDER BY a.started_at DESC
LIMIT 20;
```

**Environments by status:**
```sql
SELECT 
  status,
  COUNT(*) as count
FROM environments
GROUP BY status
ORDER BY count DESC;
```

**Attempts by status:**
```sql
SELECT 
  status,
  COUNT(*) as count
FROM attempts
GROUP BY status
ORDER BY count DESC;
```

**Orphaned environments:**
```sql
SELECT 
  e.id,
  e.status,
  a.status as attempt_status,
  e.created_at
FROM environments e
LEFT JOIN attempts a ON e.attempt_id = a.id
WHERE e.status NOT IN ('destroyed')
  AND (a.status IN ('scored', 'expired_before_start') OR a.id IS NULL)
ORDER BY e.created_at ASC;
```

### Cleanup Reconciler

The cleanup reconciler runs automatically every 60 seconds and handles:

1. **Stuck environments** - Environments in `destroying` state for > 10 minutes
2. **Pending cleanup** - Attempts in `cleanup_pending` state
3. **Terminal states** - Environments for scored/expired attempts
4. **Stuck provisioning** - Attempts in `environment_provisioning` for > 30 minutes

Monitor reconciler activity in logs:

```bash
journalctl -u painkiller | grep reconciler
```

### Job Queue Monitoring

Monitor job queue health:

**Pending jobs:**
```sql
SELECT 
  queue,
  COUNT(*) as count
FROM jobs
WHERE status = 'pending'
GROUP BY queue;
```

**Failed jobs:**
```sql
SELECT 
  id,
  queue,
  payload,
  status,
  attempts,
  last_error,
  updated_at
FROM jobs
WHERE status = 'failed'
ORDER BY updated_at DESC
LIMIT 20;
```

**Job processing rate:**
```sql
SELECT 
  queue,
  COUNT(*) FILTER (WHERE status = 'completed' AND updated_at > NOW() - INTERVAL '1 hour') as completed_last_hour,
  COUNT(*) FILTER (WHERE status = 'failed' AND updated_at > NOW() - INTERVAL '1 hour') as failed_last_hour
FROM jobs
GROUP BY queue;
```

## Operational Procedures

### Handling Failed Provisioning

1. **Check logs:**
   ```bash
   journalctl -u painkiller | grep "provision_environment"
   ```

2. **Identify the error:**
   - Proxmox API error
   - Ansible playbook failure
   - SSH connection timeout
   - Resource exhaustion

3. **Fix the issue:**
   - Check Proxmox resources
   - Verify VM templates
   - Check network connectivity
   - Review Ansible logs

4. **Retry provisioning:**
   ```bash
   curl -X POST -H "Authorization: Bearer $TOKEN" \
     http://localhost:8080/api/v1/admin/attempts/uuid/retry-provision
   ```

### Handling Stuck Environments

1. **Identify stuck environments:**
   ```sql
   SELECT * FROM environments 
   WHERE status = 'destroying' 
   AND updated_at < NOW() - INTERVAL '10 minutes';
   ```

2. **Check Proxmox for orphaned VMs:**
   ```bash
   qm list | grep painkiller
   ```

3. **Force destroy:**
   ```bash
   curl -X POST -H "Authorization: Bearer $TOKEN" \
     http://localhost:8080/api/v1/admin/environments/uuid/destroy
   ```

4. **Manual cleanup (if needed):**
   ```bash
   qm destroy <vmid> --purge
   ```

### Handling Grading Failures

1. **Check grading logs:**
   ```bash
   journalctl -u painkiller | grep "grade_attempt"
   ```

2. **Review check results:**
   ```sql
   SELECT * FROM check_results 
   WHERE attempt_id = 'uuid'
   ORDER BY ran_at DESC;
   ```

3. **Verify environment is accessible:**
   ```bash
   ssh -i /path/to/key ubuntu@<workstation_ip>
   ```

4. **Retry grading:**
   ```bash
   curl -X POST -H "Authorization: Bearer $TOKEN" \
     http://localhost:8080/api/v1/admin/attempts/uuid/retry-grade
   ```

### Emergency Shutdown

To stop all student environments:

1. **Stop the server:**
   ```bash
   sudo systemctl stop painkiller
   ```

2. **List all active environments:**
   ```bash
   curl -H "Authorization: Bearer $TOKEN" \
     http://localhost:8080/api/v1/admin/environments?status=active
   ```

3. **Destroy each environment:**
   ```bash
   for env_id in $(cat environments.txt); do
     curl -X POST -H "Authorization: Bearer $TOKEN" \
       http://localhost:8080/api/v1/admin/environments/$env_id/destroy
   done
   ```

4. **Verify cleanup in Proxmox:**
   ```bash
   qm list | grep painkiller
   ```

## Security

### Admin Access Control

- Use strong passwords for admin accounts
- Enable 2FA if available
- Limit admin accounts to essential personnel
- Audit admin actions regularly

### Audit Logging

All admin actions are logged with:
- User ID
- Action performed
- Target resource
- Timestamp
- IP address

Query audit logs:

```sql
SELECT * FROM audit_logs
WHERE user_id = 'uuid'
ORDER BY created_at DESC
LIMIT 50;
```

### API Security

- All admin endpoints require HTTPS in production
- JWT tokens expire after 24 hours
- Rate limiting recommended for admin endpoints
- Monitor for suspicious activity

## Best Practices

1. **Least privilege** - Grant admin access only when necessary
2. **Audit regularly** - Review admin actions and access patterns
3. **Document procedures** - Document operational procedures and runbooks
4. **Test backups** - Regularly test database backups
5. **Monitor resources** - Set up alerts for resource exhaustion
6. **Automate cleanup** - Rely on the cleanup reconciler for routine cleanup
7. **Staging environment** - Test admin operations in staging first
8. **Incident response** - Document incident response procedures
9. **Communication** - Notify users before maintenance or outages
10. **Post-mortems** - Conduct post-mortems for major incidents
