# Database Setup

Painkiller Shell uses PostgreSQL as its primary database. This guide covers database setup, migrations, and maintenance.

## PostgreSQL Installation

### Ubuntu/Debian

```bash
sudo apt-get update
sudo apt-get install postgresql postgresql-contrib
sudo systemctl start postgresql
sudo systemctl enable postgresql
```

### macOS (Homebrew)

```bash
brew install postgresql@14
brew services start postgresql@14
```

### Docker

```bash
docker run --name painkiller-db \
  -e POSTGRES_DB=painkiller \
  -e POSTGRES_USER=painkiller \
  -e POSTGRES_PASSWORD=password \
  -p 5432:5432 \
  -d postgres:14
```

## Database Creation

Create the database and user:

```bash
# Create database
createdb painkiller

# Or with specific owner
createuser painkiller
createdb -O painkiller painkiller

# Grant permissions (if needed)
psql painkiller -c "GRANT ALL PRIVILEGES ON DATABASE painkiller TO painkiller"
```

Verify connection:

```bash
psql "postgres://localhost:5432/painkiller?sslmode=disable" -c "SELECT 1"
```

## Running Migrations

Migrations are managed with [golang-migrate](https://github.com/golang-migrate/migrate).

### Apply All Migrations

```bash
make migrate-up
```

This runs all pending migrations in `migrations/` directory.

### Rollback Last Migration

```bash
make migrate-down
```

This rolls back the most recent migration.

### Check Migration Status

Connect to the database and check the `schema_migrations` table:

```bash
psql painkiller -c "SELECT * FROM schema_migrations"
```

## Migration Files

Migrations are stored in `migrations/` with the naming convention:

```
000001_description.up.sql
000001_description.down.sql
```

- `up.sql` files apply the migration
- `down.sql` files roll back the migration
- Numbers must be sequential
- Descriptions should be concise and descriptive

### Current Migrations

**000001_init**
- Creates core tables: `users`, `products`, `tests`, `purchased_tests`, `attempts`, `sessions`, `environments`, `clusters`, `nodes`, `jobs`
- Sets up foreign key constraints
- Creates indexes for performance

**000002_scenario_versions**
- Creates `scenario_versions`, `tasks`, `checks` tables
- Links scenarios to attempts via `scenario_version_id`

## Database Schema

### Core Tables

**users**
- User accounts with email and password hash
- `is_admin` flag for administrative access

**products**
- Stripe products with `stripe_price_id`
- Title and description for display

**tests**
- Training/exam products linked to Stripe products
- Duration, access window, and attempt limits

**purchased_tests**
- User purchases with expiry and remaining attempts
- Created via Stripe webhook after payment

**attempts**
- Individual test attempts with state machine
- Tracks status, start/end times, and scores

**sessions**
- Runtime state for attempts
- Terminal tokens for WebSocket connections

**environments**
- Infrastructure metadata (Proxmox VM IDs, IPs)
- Status tracking for provisioning lifecycle

**clusters**
- Kubernetes clusters within environments
- Context names for kubectl access

**nodes**
- VMs within clusters (control-plane or worker)
- Provider VM IDs for management

**scenario_versions**
- Immutable scenario definitions from Git
- Linked to attempts at creation time

**tasks**
- Individual questions within scenarios
- Points and prompts for students

**checks**
- Validation logic for grading
- Commands or scripts to verify task completion

**jobs**
- Async job queue (River)
- Provisioning, grading, cleanup tasks

## Database Maintenance

### Backup

Create a backup:

```bash
pg_dump painkiller > backup_$(date +%Y%m%d).sql
```

Restore from backup:

```bash
psql painkiller < backup_20260529.sql
```

### Vacuum and Analyze

Optimize database performance:

```bash
psql painkiller -c "VACUUM ANALYZE"
```

### Index Usage

Check index usage statistics:

```sql
SELECT
  schemaname,
  tablename,
  indexname,
  idx_scan,
  idx_tup_read,
  idx_tup_fetch
FROM pg_stat_user_indexes
ORDER BY idx_scan DESC;
```

### Slow Queries

Find slow queries:

```sql
SELECT
  query,
  calls,
  total_time,
  mean_time,
  rows
FROM pg_stat_statements
ORDER BY mean_time DESC
LIMIT 10;
```

## Connection Pooling

The application uses connection pooling via `sqlx`. Default pool settings:

- Max open connections: 25
- Max idle connections: 5
- Connection max lifetime: 5 minutes

Adjust in `internal/store/store.go` if needed for your workload.

## Troubleshooting

### Connection Refused

Ensure PostgreSQL is running:

```bash
systemctl status postgresql
# or
brew services list | grep postgresql
```

### Authentication Failed

Check `pg_hba.conf` for authentication rules:

```bash
# Find config file location
psql -c "SHOW hba_file"

# Edit and reload
sudo systemctl reload postgresql
```

### Database Does Not Exist

Create the database:

```bash
createdb painkiller
```

### Migration Failed

Check the error message and fix the issue. Common causes:
- Syntax error in migration SQL
- Foreign key constraint violation
- Duplicate table or column

To retry after fixing:

```bash
make migrate-down  # Roll back failed migration
make migrate-up    # Re-apply
```

### Permission Denied

Grant necessary permissions:

```sql
GRANT ALL PRIVILEGES ON DATABASE painkiller TO painkiller;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO painkiller;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO painkiller;
```

## Production Considerations

### SSL/TLS

Enable SSL in `DATABASE_URL`:

```bash
DATABASE_URL=postgres://user:pass@host:5432/painkiller?sslmode=require
```

Configure PostgreSQL for SSL in `postgresql.conf`:

```
ssl = on
ssl_cert_file = '/path/to/server.crt'
ssl_key_file = '/path/to/server.key'
```

### Connection Limits

Set appropriate connection limits in `postgresql.conf`:

```
max_connections = 100
```

Ensure your application pool size × number of app instances < max_connections.

### Monitoring

Monitor database health with:

```sql
-- Active connections
SELECT count(*) FROM pg_stat_activity;

-- Database size
SELECT pg_size_pretty(pg_database_size('painkiller'));

-- Table sizes
SELECT
  schemaname,
  tablename,
  pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename))
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
```
