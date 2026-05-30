# AGENTS.md

## Project State

- `go.mod` is the only project manifest; module path is `painkiller-shell` (a.k.a Painkiller, Painkiller Shell) and it declares Go `1.26.1`.
- No Go packages, entrypoints, tests, README, CI, task runner, or lockfile exist yet.
- Product intent from repo notes: Painkiller Shell is a Killer.sh clone written in Go.
- Architecture decisions and MVP scope are documented in `docs/architecture.md`.

## Commands

- Development debugging is primarily done with `make run-dev`, which runs the ephemeral Docker Compose stack from `resources/docker-compose-dev-ephemeral.yaml`.
- When debugging pasted Painkiller log messages, assume the environment information, env vars, service parameters, ports, and defaults from `resources/docker-compose-dev-ephemeral.yaml` unless the user says otherwise.
- Once Go files exist, prefer focused verification with `go test ./...` unless a narrower package test is enough.

## Workflow Notes

- Keep changes minimal; avoid broad rewrites when a targeted edit solves the task.
- When the user asks to persist useful context for future sessions, add it to `AGENTS.md` or another discoverable `.md` file.
- Squid is an external infrastructure component. Painkiller config may contain only the proxy address needed to configure student workstations; Squid ACLs, allowlists, caching, auth, and filtering policy belong in Squid/infra config, not Go app env vars.

## Documentation

- All code changes must be followed by an update to the user-facing docs in `./docsite`.
- When adding new features, API endpoints, config options, or operational procedures, update the relevant docsite page(s) in the same change.
- `./docsite` is a Hugo content directory. Use `make docs-serve` to preview and `make docs-build` to build the static site into ignored `./public` output.
