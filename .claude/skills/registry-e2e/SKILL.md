---
name: registry-e2e
description: Run the Robot Agent Registry end-to-end acceptance test (spec §5) locally — owner register, Ed25519 login, platform register, rar-agent start, and online discovery — against a throwaway Postgres + server + Python CLI. Use to validate registry changes before deploying.
---

# registry-e2e

Validate the registry end to end the way the spec §5 acceptance scenario does.

## Run it

```bash
./scripts/e2e-local.sh
```

The script is self-contained and cleans up after itself. It:

1. Spins up a throwaway Postgres cluster (initdb + pg_ctl) on `PGPORT` (55434).
2. Builds and runs the server on `APIPORT` (8090), applying migrations on boot.
3. Installs the Python CLI (`./sdk/python`) into a temp venv.
4. Walks `rar auth register` → `rar auth login` → `rar platform register` →
   `rar-agent start --preset operations` → `rar discover agents --online`.
5. Asserts the agent and its `get_system_status` capability are discoverable
   online, prints `PASS`, and exits non-zero on any failure.

Override ports with `PGPORT=... APIPORT=... ./scripts/e2e-local.sh` if they clash.

## Requirements

`go`, `python3`, and Postgres client tools (`initdb`, `pg_ctl`, `createdb`) on PATH.
On macOS: `brew install postgresql@16`.

## When debugging a failure

- Server log and agent log are under the temp `WORK` dir (printed paths) — but the
  dir is removed on exit; comment out the `trap cleanup EXIT` line to inspect.
- Unit + API tests (no full flow) run faster: `go test ./...` with
  `TEST_DATABASE_URL` set to a running Postgres.
