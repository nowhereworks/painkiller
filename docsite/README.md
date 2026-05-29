# Painkiller Shell Documentation

Painkiller Shell is a Killer.sh-style Kubernetes training platform. Students purchase tests, receive time-limited access with a fixed number of attempts, and train in live kubeadm-based environments running on Proxmox VMs.

## Documentation Index

### Getting Started
- [Getting Started](getting-started.md) - Prerequisites, installation, and quick start guide
- [Configuration](configuration.md) - Environment variables and configuration reference
- [Database Setup](database.md) - PostgreSQL setup and database migrations

### Operations
- [Running the Server](running.md) - Building, running, testing, and monitoring
- [Admin Controls](admin.md) - Administrative API endpoints and monitoring

### Infrastructure
- [Proxmox Setup](proxmox.md) - Proxmox API tokens, VM templates, and network configuration
- [Documentation Proxy](proxy.md) - Deploying and managing the restricted documentation proxy

### Content Authoring
- [Scenario Authoring](scenarios.md) - Creating and validating training scenarios

### Support
- [Troubleshooting](troubleshooting.md) - Common issues and resolutions

### Legal
- [Licensing](license.md) - Personal-use terms and commercial licensing requirements

## Architecture Overview

Painkiller Shell follows a modular monolith architecture:

```
Next.js Frontend
  ↓
Go API Server
  ├── Auth (JWT)
  ├── Stripe Billing
  ├── Test Catalog & Entitlements
  ├── Attempt Lifecycle
  ├── Terminal Gateway (WebSocket → SSH)
  ├── Grading Engine
  └── Job Queue (PostgreSQL-backed)
       ├── Proxmox Provider
       └── Ansible Provisioner
```

Each student attempt provisions an isolated environment containing:
- Student workstation VM (terminal access point)
- One or more kubeadm clusters
- Restricted documentation proxy access
- Ephemeral SSH keys and network isolation

## Key Concepts

- **Product**: A Stripe-sellable item
- **Test**: A training or exam product (e.g., "CKA Simulator 1")
- **Purchased Test**: A user's time-limited access to a test with N attempts
- **Attempt**: One try at a purchased test
- **Session**: Runtime state for an attempt (terminal token, etc.)
- **Environment**: Infrastructure created for an attempt (VMs, clusters)
- **Scenario**: Git-versioned test definition (topology, tasks, checks)

## Support

For issues and feature requests, visit the [GitHub repository](https://github.com/your-org/painkiller-shell).
