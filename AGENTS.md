# AGENTS.md

## Project State

- `go.mod` is the only project manifest; module path is `painkiller-shell` and it declares Go `1.26.1`.
- No Go packages, entrypoints, tests, README, CI, task runner, or lockfile exist yet.
- Product intent from repo notes: Painkiller Shell is a Killer.sh clone written in Go.
- Architecture decisions and MVP scope are documented in `docs/architecture.md`.

## Commands

- There are no repo-specific scripts yet; use direct Go commands as files are added.
- Once Go files exist, prefer focused verification with `go test ./...` unless a narrower package test is enough.

## Workflow Notes

- Keep changes minimal; avoid broad rewrites when a targeted edit solves the task.
- When the user asks to persist useful context for future sessions, add it to `AGENTS.md` or another discoverable `.md` file.
