# Getting Started

This guide walks you through setting up Painkiller Shell from scratch.

## Prerequisites

### Required Software

- **Go 1.26.1 or later** - [Install Go](https://go.dev/doc/install)
- **Node.js 22 or later** - Required for the embedded Next.js UI build
- **PostgreSQL 14+** - [Install PostgreSQL](https://www.postgresql.org/download/)
- **Make** - Usually pre-installed on Linux/macOS

### Infrastructure (for production)

- **Proxmox VE 7+** - For VM provisioning and management
- **Ansible 2.9+** - For kubeadm cluster provisioning
- **Squid Proxy** - For restricted documentation access
- **Stripe Account** - For payment processing

### Optional (for development)

- **Docker** - For running PostgreSQL in a container
- **Stripe CLI** - For testing webhooks locally

## Quick Start

### 1. Clone the Repository

```bash
git clone https://github.com/your-org/painkiller-shell.git
cd painkiller-shell
```

### 2. Install Dependencies

```bash
go mod download
npm --prefix web install
```

### 3. Configure Environment

Copy the example environment file and edit it:

```bash
cp .env.example .env
```

Edit `.env` and set at minimum:

```bash
DATABASE_URL=postgres://localhost:5432/painkiller?sslmode=disable
JWT_SECRET=change-me-to-a-random-secret
HTTP_ADDR=:8080
```

See [Configuration](configuration.md) for all available options.

### 4. Set Up the Database

Create the PostgreSQL database:

```bash
createdb painkiller
```

Run migrations:

```bash
make migrate-up
```

Verify tables were created:

```bash
psql painkiller -c "\dt"
```

See [Database Setup](database.md) for detailed instructions.

### 5. Run the Server

```bash
make run
```

The server starts on `http://localhost:8080`. Test the health endpoint:

```bash
curl http://localhost:8080/healthz
# {"status":"ok"}
```

The embedded UI is served from the same address after running `make web-build` and rebuilding the Go server. During frontend development, run the Next.js dev server separately:

```bash
make web-dev
```

The dev UI runs at `http://127.0.0.1:3000` and proxies `/api/*` to the Go API on `http://localhost:8080` by default. Override with `NEXT_PUBLIC_API_BASE_URL` if needed.

## Next Steps

### For Development

1. **Run tests**: `make test`
2. **Run linter**: `make lint`
3. **Build the embedded UI**: `make web-build`
4. **Create a test user**: Use the registration UI or API endpoint
5. **Set up Stripe**: Configure Stripe keys and test webhooks
6. **Use mock provider**: Development uses a mock Proxmox provider by default

### For Production

1. **Set up Proxmox**: Configure API tokens and VM templates - see [Proxmox Setup](proxmox.md)
2. **Deploy documentation proxy**: Set up Squid proxy - see [Documentation Proxy](proxy.md)
3. **Create scenarios**: Author training scenarios - see [Scenario Authoring](scenarios.md)
4. **Configure admin access**: Create admin users for operational control - see [Admin Controls](admin.md)

## API Endpoints

Once running, the following endpoints are available:

### Public Endpoints
- `GET /healthz` - Health check
- `GET /api/v1/catalog/tests` - List available tests
- `POST /api/v1/auth/register` - Register new user
- `POST /api/v1/auth/login` - Login and get JWT
- `POST /api/v1/auth/logout` - Clear auth cookie

### Authenticated Endpoints
- `POST /api/v1/billing/checkout` - Create Stripe checkout session
- `GET /api/v1/entitlements/dashboard` - List purchased tests
- `POST /api/v1/attempts` - Start a test attempt
- `GET /api/v1/attempts/:id` - Get attempt status
- `WS /api/v1/terminal/:token` - WebSocket terminal connection
- `POST /api/v1/attempts/:id/submit` - Submit attempt for grading
- `GET /api/v1/scoring/attempts/:id/score` - Get attempt score

### Webhooks
- `POST /api/v1/webhooks/stripe` - Stripe webhook endpoint

## Development Workflow

```bash
# Run the server with hot reload (requires air: go install github.com/cosmtrek/air@latest)
air

# Run tests
make test

# Run linter
make lint

# Run database migrations
make migrate-up
make migrate-down

# Run the frontend dev server
make web-dev

# Build the frontend for embedding in the Go binary
make web-build

# Build binary
go build -o server ./cmd/server
```

## Common Issues

### Database Connection Failed

Ensure PostgreSQL is running and `DATABASE_URL` is correct:

```bash
psql "postgres://localhost:5432/painkiller?sslmode=disable" -c "SELECT 1"
```

### Port Already in Use

Change `HTTP_ADDR` in `.env`:

```bash
HTTP_ADDR=:8081
```

### Migrations Fail

Ensure the database exists and you have write permissions:

```bash
createdb painkiller
psql painkiller -c "GRANT ALL PRIVILEGES ON DATABASE painkiller TO $USER"
```

See [Troubleshooting](troubleshooting.md) for more common issues.
