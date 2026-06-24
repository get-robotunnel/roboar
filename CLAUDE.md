# CLAUDE.md — Robot Agent Registry

Reference implementation of the Robot Agent Registry (RAR) protocol. Open-source
(Apache-2.0). Kept physically separate from the commercial RoboTunnel Operations
platform.

## Source of truth

The authoritative spec is `robot-agent-registry-spec.md`, located in the sibling
workspace at `../robotunnel/robot-agent-registry-spec.md` (one directory up from
this repo, under `Documents/AI/Code/robotunnel/`). Strategic positioning is in
`../robotunnel/project-grind.md`. **When in doubt, the spec wins.** If you change
behavior that diverges from the spec, update the spec too.

## What this is / isn't

- IS: an HTTP API storing identity + discovery + capability metadata + payment
  terms. Phase 1 = Owner/Platform/Agent/Capability registration, Ed25519 login,
  heartbeat, public Discovery API.
- IS NOT: a traffic relay (data flows over RoboTunnel tunnel; we store only
  `tunnel_endpoint`) and IS NOT a payment gateway (settlement is x402; we store
  only payment terms). See spec §9.

## Layout

```
cmd/registry/main.go   server entry (Gin, :8090, auto-runs migrations)
internal/server        router + middleware
internal/auth          ed25519 verify · JWT (golang-jwt/v5) · bcrypt
internal/owners        register, challenge/verify → JWT, /me
internal/platforms     register → platform_token (returned once), heartbeat
internal/agents        agent + capability CRUD
internal/discovery     public GET /discover/* (no auth)
internal/store         pgxpool + embedded migration runner
internal/ids           nanoid prefixes usr_/plt_/agt_/cap_
db/migrations/*.sql    schema (embedded, applied in order on boot)
sdk/python             rar-agent (platform-side SDK)
cli                    rar-cli (owner-side CLI)
```

## Conventions

- Go 1.25, Gin, `jackc/pgx/v5` (no Supabase SDK — plain Postgres for self-hosting).
- IDs are prefixed nanoids via `internal/ids`.
- `platform_token` plaintext is returned **only** at creation; DB stores bcrypt hash.
- All IDs/secrets out of logs.
- Base URL is config-driven (`REGISTRY_BASE_URL`), defaults to
  `https://reg.robotunnel.io/v1`.

## Commands

```bash
go build ./...                 # compile
go test ./...                  # unit + httptest
go run ./cmd/registry          # local server (needs DATABASE_URL, JWT_SIGNING_KEY)
GOOS=linux GOARCH=amd64 go build -o robot-agent-registry ./cmd/registry  # release
```

Required env: `DATABASE_URL`, `JWT_SIGNING_KEY`. Optional: `PORT` (default 8090),
`REGISTRY_BASE_URL`, `HEARTBEAT_OFFLINE_SECS` (default 60).

## Deploy

Target `reg.robotunnel.io` on the shared VPS, port 8090, behind Caddy, systemd
unit, separate Supabase/Postgres project from Operations. See `deploy/` and the
`deploy-registry` skill. Migrations apply automatically on startup.

## Phase boundaries (don't implement ahead of the milestone)

Phase 1 (now): entities, Ed25519 login, registration, heartbeat, Discovery API,
Python SDK + CLI. Phase 2+: WebSocket config push, Agent Tokens, Discovery Web UI,
MCP server, x402 metering, ROS2 capability execution. Don't build Phase 2+ unless
asked.
