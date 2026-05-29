<p align="center">
  <a href="docs/painkiller.png">
    <img src="docs/painkiller.png" alt="Painkiller" width="502">
  </a>
</p>

# Painkiller

Painkiller helps learners prepare for Kubernetes certifications without manually building disposable lab environments. A student buys or unlocks a test, starts an attempt, receives an isolated workstation and one or more Kubernetes clusters, completes scenario tasks, then submits the attempt for grading.

Core product concepts include products, tests, purchased access windows, attempts, sessions, environments, tasks, and checks. Infrastructure details stay behind provider and provisioning boundaries so the application can focus on training flows, scoring, and retakes.

## Repo Layout

- `cmd/` contains executable entrypoints, including the API server and database migration runner.
- `internal/` contains private Go application packages for auth, billing, catalog, entitlements, attempts, terminal access, grading, jobs, providers, provisioning, storage, and shared HTTP utilities.
- `migrations/` contains PostgreSQL schema migrations used by the migration command.
- `infra/` contains operational infrastructure files, including proxy configuration and workstation network setup scripts.
- `testdata/` contains test fixtures, including sample scenario definitions used by Go tests.
- `sample/` is reserved for sample scenario content.
- `docs/` contains architecture notes, implementation planning, and project assets.
- `docsite/` contains user-facing documentation for setup, configuration, operations, infrastructure, scenario authoring, and troubleshooting.

## How to Get Started

Start by installing the required dependencies, copying `.env.example` to `.env`, configuring PostgreSQL and application secrets, running migrations, and starting the Go API server with `make run`.

For the full setup flow, see [Getting Started](docsite/getting-started.md).

## License

Painkiller Shell is source-available under the [Painkiller Business Source License](LICENSE). It is free for personal, non-commercial use only. Commercial use, business use, hosted services, paid services, managed services, SaaS offerings, training services, and derivative commercial offerings require a separate written license from Nowhereworks.

For a user-facing summary, see [Licensing](docsite/license.md).
