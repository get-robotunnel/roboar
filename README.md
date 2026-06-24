# Robot Agent Registry (RAR)

> Open-source registry that lets agents running on physical robots and human-side
> agents (laptop / phone) discover each other on the public internet, establish
> connections, invoke capabilities, and be managed by their owners.

RAR is **infrastructure, not a product UI** — its core form is an HTTP API (plus,
later, an MCP server). It stores *identity + discovery + capability metadata +
payment terms*. It deliberately does **not** relay traffic or custody funds:

- Connection data flows over **RoboTunnel tunnel**; RAR only stores the
  `tunnel_endpoint`.
- Payment settles over **x402** (USDC on Base); RAR only stores payment terms.

This repository is the **reference implementation** of the registry protocol
defined in `robot-agent-registry-spec.md` (kept in the adjacent `robotunnel/`
workspace; the spec is the source of truth — see `CLAUDE.md` for the pointer).

## Status

Phase 1 (MVP). Implements: Owner registration + Ed25519 challenge-response login,
Platform registration + heartbeat, Agent + Capability registration, and the public
Discovery API. Deployed at `https://reg.robotunnel.io/v1`.

Not yet implemented (Phase 2/3): WebSocket config push, Agent Tokens, Discovery Web
UI, MCP server, x402 metering, ROS2 capability execution, on-chain identity.

## Architecture

```
cmd/registry         Gin HTTP API server (:8090)
internal/server      router + middleware (auth, CORS, recovery)
internal/auth        Ed25519 verify · JWT (golang-jwt) · bcrypt platform_token
internal/owners      POST /owners, challenge/verify login → JWT
internal/platforms   register → platform_token (once), heartbeat
internal/agents      agent + capability CRUD
internal/discovery   public GET /discover/* (no auth)
internal/store       pgxpool access to Postgres
internal/ids         nanoid helpers (usr_/plt_/agt_/cap_)
db/migrations        SQL schema (applied on startup)
sdk/python           rar-agent (platform-side SDK)
cli                  rar-cli (owner-side CLI)
```

## Quick start (local)

```bash
createdb rar
export DATABASE_URL="postgres://localhost:5432/rar?sslmode=disable"
export JWT_SIGNING_KEY="$(openssl rand -hex 32)"
go run ./cmd/registry          # migrations auto-apply on boot; listens on :8090

curl http://localhost:8090/v1/discover/agents   # → {"agents":[],"total":0,...}
```

## Self-hosting

RAR needs only a Postgres database and one binary. Set `DATABASE_URL`,
`JWT_SIGNING_KEY`, `PORT`, and `REGISTRY_BASE_URL`, then run the binary behind any
TLS reverse proxy.

## License

Apache-2.0. See [LICENSE](./LICENSE).
