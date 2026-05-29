# Configuration

Painkiller Shell uses environment variables for configuration. Copy `.env.example` to `.env` and adjust values as needed.

## Configuration Reference

### Database

**`DATABASE_URL`** (required)
- PostgreSQL connection string
- Format: `postgres://user:password@host:port/database?sslmode=disable`
- Example: `postgres://localhost:5432/painkiller?sslmode=disable`
- The database must exist before running migrations

### HTTP Server

**`HTTP_ADDR`** (required)
- Address and port for the HTTP server
- Format: `:port` or `host:port`
- Default: `:8080`
- Example: `:8080` or `0.0.0.0:8080`

### Authentication

**`JWT_SECRET`** (required)
- Secret key for JWT token signing
- Use a strong random value (32+ characters)
- Generate with: `openssl rand -hex 32`
- Example: `a1b2c3d4e5f6...`

**`JWT_EXPIRY`** (optional)
- JWT token expiry in seconds
- Default: `86400` (24 hours)
- Example: `86400`

### Stripe Billing

**`STRIPE_SECRET_KEY`** (required for billing)
- Stripe API secret key
- Format: `sk_live_...` or `sk_test_...`
- Get from Stripe Dashboard → Developers → API keys

**`STRIPE_WEBHOOK_SECRET`** (required for billing)
- Stripe webhook signing secret
- Format: `whsec_...`
- Get from Stripe Dashboard → Developers → Webhooks → your endpoint

**`STRIPE_SUCCESS_URL`** (required for billing)
- URL to redirect after successful payment
- Example: `http://localhost:3000/success`
- This is your frontend app's success page

**`STRIPE_CANCEL_URL`** (required for billing)
- URL to redirect after cancelled payment
- Example: `http://localhost:3000/cancel`
- This is your frontend app's cancel page

### Proxmox Integration

**`PROXMOX_URL`** (required for Proxmox provider)
- Proxmox API endpoint URL
- Format: `https://host:port`
- Example: `https://proxmox.example.com:8006`

**`PROXMOX_TOKEN_ID`** (required for Proxmox provider)
- Proxmox API token ID
- Format: `user@realm!tokenname`
- Example: `painkiller@pam!api`
- See [Proxmox Setup](proxmox.md) for token creation

**`PROXMOX_TOKEN_SECRET`** (required for Proxmox provider)
- Proxmox API token secret
- Get from Proxmox when creating the token

**`PROXMOX_NODE`** (required for Proxmox provider)
- Proxmox node name where VMs will be created
- Example: `pve1`
- Get from Proxmox UI → Datacenter → Nodes

### Scenario Management

**`SCENARIO_REPO_PATH`** (required for scenarios)
- Absolute path to the Git repository containing scenarios
- Example: `/opt/painkiller-scenarios`
- See [Scenario Authoring](scenarios.md) for repo structure

### Logging

**`LOG_LEVEL`** (optional)
- Logging verbosity level
- Values: `debug`, `info`, `warn`, `error`
- Default: `info`
- Example: `debug`

## Environment-Specific Configuration

### Development

Minimal configuration for local development:

```bash
DATABASE_URL=postgres://localhost:5432/painkiller?sslmode=disable
HTTP_ADDR=:8080
JWT_SECRET=dev-secret-change-in-production
LOG_LEVEL=debug
```

Development uses a mock Proxmox provider, so Proxmox credentials are not required.

### Production

Full configuration for production deployment:

```bash
DATABASE_URL=postgres://user:pass@db.example.com:5432/painkiller?sslmode=require
HTTP_ADDR=:8080
JWT_SECRET=<strong-random-secret>
JWT_EXPIRY=86400

STRIPE_SECRET_KEY=sk_live_...
STRIPE_WEBHOOK_SECRET=whsec_...
STRIPE_SUCCESS_URL=https://app.example.com/success
STRIPE_CANCEL_URL=https://app.example.com/cancel

PROXMOX_URL=https://proxmox.example.com:8006
PROXMOX_TOKEN_ID=painkiller@pam!api
PROXMOX_TOKEN_SECRET=<secret>
PROXMOX_NODE=pve1

SCENARIO_REPO_PATH=/opt/painkiller-scenarios
LOG_LEVEL=info
```

## Loading Configuration

Configuration is loaded in this order (later sources override earlier):

1. Default values (hardcoded)
2. `.env` file (if present)
3. Environment variables

The server validates required configuration on startup and exits with an error if required values are missing.

## Secrets Management

For MVP, secrets are stored in environment variables via `.env` file. For production:

- Use a secrets manager (AWS Secrets Manager, HashiCorp Vault, etc.)
- Inject secrets via environment variables from your deployment platform
- Never commit `.env` files with real secrets to version control

## Configuration Validation

The server validates configuration on startup:

- `DATABASE_URL` must be a valid PostgreSQL connection string
- `JWT_SECRET` must be at least 32 characters
- `STRIPE_SECRET_KEY` must start with `sk_` if provided
- `PROXMOX_URL` must be a valid HTTPS URL if provided

Invalid configuration causes the server to exit with a descriptive error message.

## Updating Configuration

After changing `.env`, restart the server:

```bash
# If running with make run
Ctrl+C
make run

# If running as a service
systemctl restart painkiller
```

Configuration changes require a server restart to take effect.
