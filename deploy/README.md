# Deploying the Robot Agent Registry

The registry runs as its own service at `reg.robotunnel.io`, on the same VPS as
the Operations platform but on a **separate port (8090)**, behind Caddy, with its
**own Postgres database** (do not share the Operations Supabase project).

## Prerequisites (one-time,需要人工)

These steps require credentials/DNS access and are **not** automated:

1. **DNS** — add an A record `reg.robotunnel.io → <VPS IP>`.
2. **Database** — create a dedicated Postgres (e.g. a new Supabase project) and
   copy its connection string. Schema is applied automatically on first boot
   (embedded migrations), so no manual SQL is needed.
3. **JWT key** — `openssl rand -hex 32` for `JWT_SIGNING_KEY`.
4. **GitHub secrets** (in a `production` environment) for the deploy workflow:
   `PROD_SSH_HOST`, `PROD_SSH_PORT`, `PROD_SSH_USER`, `PROD_SSH_PRIVATE_KEY`,
   `PROD_SSH_KNOWN_HOSTS`.

## First-time VPS setup

```bash
scp deploy/bootstrap.sh root@<vps>:/tmp/
ssh root@<vps> 'bash /tmp/bootstrap.sh'
# then edit /opt/robot-agent-registry/config/.env: set DATABASE_URL + JWT_SIGNING_KEY
```

`bootstrap.sh` creates the `robotunnel` user, the config dir + `.env` template,
and appends the `reg.robotunnel.io` vhost to the Caddyfile.

## Deploying the binary

**Preferred:** run the **Deploy Registry** GitHub Action (`workflow_dispatch`,
pick `goarch` to match the VPS — `amd64` by default). It builds, uploads, and runs
`setup.sh`. It never overwrites `.env`.

**Manual:**

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o robot-agent-registry ./cmd/registry
scp robot-agent-registry deploy/registry.service deploy/setup.sh root@<vps>:/tmp/
ssh root@<vps> 'cd /tmp && ./setup.sh'
```

## Verify

```bash
curl -s https://reg.robotunnel.io/v1/discover/agents      # → {"agents":[],"total":0,...}
ssh root@<vps> 'systemctl status robot-agent-registry'    # active (running)
```

Then run the full owner→platform→agent→discover walkthrough — see the
`registry-e2e` skill or `sdk/python/README.md`.
