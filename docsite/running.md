# Running the Server

This guide covers building, running, testing, and monitoring the Painkiller Shell server.

## Development

### Run with Hot Reload

Install [air](https://github.com/cosmtrek/air) for automatic rebuilds:

```bash
go install github.com/cosmtrek/air@latest
```

Run with hot reload:

```bash
air
```

The server automatically rebuilds and restarts when code changes.

### Run Without Hot Reload

```bash
make run
```

This runs `go run ./cmd/server` directly.

### Run Tests

Run all tests:

```bash
make test
```

Run tests for a specific package:

```bash
go test ./internal/auth/...
```

Run tests with coverage:

```bash
go test -cover ./...
```

Run tests with verbose output:

```bash
go test -v ./...
```

### Run Linter

```bash
make lint
```

This runs `go vet ./...` to check for common issues.

## Building

### Build Binary

```bash
go build -o server ./cmd/server
```

The binary is created in the current directory.

### Build for Production

Build with optimizations:

```bash
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server ./cmd/server
```

### Build Migration Tool

```bash
go build -o migrate ./cmd/migrate
```

## Running

### Start Server

Run the binary directly:

```bash
./server
```

Or with make:

```bash
make run
```

The server starts on the address specified in `HTTP_ADDR` (default `:8080`).

### Graceful Shutdown

The server handles `SIGINT` and `SIGTERM` for graceful shutdown:

- Stops accepting new connections
- Waits for in-flight requests to complete (up to 30 seconds)
- Closes database connections
- Exits cleanly

Send shutdown signal:

```bash
# If running in foreground
Ctrl+C

# If running as background process
kill -SIGTERM <pid>
```

### Background Execution

Run as a background process:

```bash
nohup ./server > server.log 2>&1 &
echo $! > server.pid
```

Stop the server:

```bash
kill $(cat server.pid)
```

## Systemd Service

Create `/etc/systemd/system/painkiller.service`:

```ini
[Unit]
Description=Painkiller Shell Server
After=network.target postgresql.service

[Service]
Type=simple
User=painkiller
WorkingDirectory=/opt/painkiller-shell
EnvironmentFile=/opt/painkiller-shell/.env
ExecStart=/opt/painkiller-shell/server
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable painkiller
sudo systemctl start painkiller
```

Check status:

```bash
sudo systemctl status painkiller
```

View logs:

```bash
sudo journalctl -u painkiller -f
```

## Docker

### Build Image

Create `Dockerfile`:

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .
COPY --from=builder /app/migrations ./migrations
EXPOSE 8080
CMD ["./server"]
```

Build and run:

```bash
docker build -t painkiller-shell .
docker run -p 8080:8080 --env-file .env painkiller-shell
```

### Docker Compose

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  db:
    image: postgres:14
    environment:
      POSTGRES_DB: painkiller
      POSTGRES_USER: painkiller
      POSTGRES_PASSWORD: password
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data

  server:
    build: .
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: postgres://painkiller:password@db:5432/painkiller?sslmode=disable
      HTTP_ADDR: :8080
      JWT_SECRET: change-me-in-production
    depends_on:
      - db

volumes:
  postgres_data:
```

Run with Docker Compose:

```bash
docker-compose up -d
```

## Health Checks

### Health Endpoint

Check server health:

```bash
curl http://localhost:8080/healthz
# {"status":"ok"}
```

Use this endpoint for load balancer health checks and monitoring.

### Database Connectivity

The health endpoint does not check database connectivity. To verify database access:

```bash
curl http://localhost:8080/api/v1/tests
```

This endpoint queries the database and returns an error if the connection fails.

## Logging

### Log Levels

Configure via `LOG_LEVEL` environment variable:

- `debug` - Detailed information for debugging
- `info` - General operational information (default)
- `warn` - Warning conditions
- `error` - Error conditions only

### Log Format

Logs are structured JSON using `log/slog`:

```json
{"time":"2026-05-29T10:00:00Z","level":"INFO","msg":"server started","addr":":8080"}
{"time":"2026-05-29T10:00:01Z","level":"INFO","msg":"request","method":"GET","path":"/healthz","status":200,"duration":"1.2ms"}
```

### Log Destinations

By default, logs write to stdout. To write to a file:

```bash
./server > /var/log/painkiller.log 2>&1
```

Or configure your process manager (systemd, Docker) to capture logs.

## Monitoring

### Metrics

The server does not expose metrics by default. To add Prometheus metrics, integrate a library like `prometheus/client_golang`.

### Request Tracing

Enable debug logging to trace requests:

```bash
LOG_LEVEL=debug ./server
```

### Error Tracking

Monitor logs for errors:

```bash
# With systemd
journalctl -u painkiller -p err

# With log file
grep '"level":"ERROR"' /var/log/painkiller.log
```

## Performance Tuning

### Database Connections

Adjust connection pool size in `internal/store/store.go`:

```go
db.SetMaxOpenConns(50)  // Increase from default 25
db.SetMaxIdleConns(10)  // Increase from default 5
```

### HTTP Server

Adjust timeouts in `internal/httpx/server.go`:

```go
server := &http.Server{
  ReadTimeout:  10 * time.Second,
  WriteTimeout: 30 * time.Second,
  IdleTimeout:  120 * time.Second,
}
```

### Job Queue

Adjust worker concurrency in `internal/jobs/worker.go`:

```go
worker := river.NewWorker(queue, river.WorkerConfig{
  MaxWorkers: 10,  // Increase from default
})
```

## Deployment Checklist

Before deploying to production:

- [ ] Set strong `JWT_SECRET` (32+ random characters)
- [ ] Configure production `DATABASE_URL` with SSL
- [ ] Set up Stripe live keys and webhook endpoint
- [ ] Configure Proxmox API credentials
- [ ] Set `SCENARIO_REPO_PATH` to production scenario repo
- [ ] Set `LOG_LEVEL=info` or `warn`
- [ ] Configure systemd service or container orchestration
- [ ] Set up log aggregation (ELK, Loki, CloudWatch)
- [ ] Configure monitoring and alerting
- [ ] Test graceful shutdown
- [ ] Verify database backups
- [ ] Load test the application
- [ ] Set up SSL/TLS termination (nginx, load balancer)
- [ ] Configure firewall rules
- [ ] Document operational procedures
