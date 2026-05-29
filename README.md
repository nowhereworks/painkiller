<p align="center">
  <a href="docs/painkiller.png">
    <img src="docs/painkiller.png" alt="Painkiller" width="502">
  </a>
</p>

# Painkiller

Painkiller helps learners prepare for Kubernetes certifications without manually building disposable lab environments. A student buys or unlocks a test, starts an attempt, receives an isolated workstation and one or more Kubernetes clusters, completes scenario tasks, then submits the attempt for grading.

Core product concepts include products, tests, purchased access windows, attempts, sessions, environments, tasks, and checks. Infrastructure details stay behind provider and provisioning boundaries so the application can focus on training flows, scoring, and retakes.

## Repo Layout

| Path | Purpose |
| --- | --- |
| `cmd/` | Executable entrypoints, including the API server and database migration runner. |
| `internal/` | Private Go application packages for auth, billing, catalog, entitlements, attempts, terminal access, grading, jobs, providers, provisioning, storage, and shared HTTP utilities. |
| `migrations/` | PostgreSQL schema migrations used by the migration command. |
| `infra/` | Operational infrastructure files, including proxy configuration and workstation network setup scripts. |
| `testdata/` | Test fixtures, including sample scenario definitions used by Go tests. |
| `sample/` | Reserved for sample scenario content. |
| `docs/` | Architecture notes, implementation planning, and project assets. |
| `docsite/` | User-facing documentation for setup, configuration, operations, infrastructure, scenario authoring, and troubleshooting. |

## How to Get Started

Painkiller is a full training-platform stack, so the first setup step is making sure you have the local application pieces and, when you are ready for real lab provisioning, the infrastructure pieces.

For local development you need:

- **Go 1.26.1 or later** for the API server and migration tooling.
- **PostgreSQL 14+** for application state, attempts, jobs, and scenario metadata.
- **Make** for the project command shortcuts.
- **Docker** if you prefer to run PostgreSQL locally in a container.
- **Stripe CLI** if you want to exercise billing webhooks during development.

For production-style environments you also need:

- **Proxmox VE 7+** to create the isolated workstation and Kubernetes cluster VMs.
- **Ansible 2.9+** to provision kubeadm-based clusters inside those VMs.
- **Squid Proxy** to provide restricted documentation access during attempts.
- **Stripe account and API keys** to sell tests and unlock purchased access windows.

Once those pieces are available, the setup path is straightforward: create your environment file from `.env.example`, point Painkiller at PostgreSQL, set application secrets, run the database migrations, and start the API server. Local development can use the mock provider while you build out catalog, entitlement, attempt, and grading flows; production deployments connect the same application flow to Proxmox, Ansible, the documentation proxy, and Stripe.

For the full setup flow, see [Getting Started](docsite/getting-started.md).

## Documentation Site

The user-facing documentation in `docsite/` can be served as a Hugo static site:

```bash
make docs-serve
```

Build the static site into `public/`:

```bash
make docs-build
```

## License

Painkiller Shell is source-available under the [Painkiller Business Source License](LICENSE). It is free for personal, non-commercial use only. Commercial use, business use, hosted services, paid services, managed services, SaaS offerings, training services, and derivative commercial offerings require a separate written license from Nowhereworks.

For a user-facing summary, see [Licensing](docsite/license.md).
