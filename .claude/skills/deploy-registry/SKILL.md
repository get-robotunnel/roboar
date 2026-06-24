---
name: deploy-registry
description: Build and deploy the Robot Agent Registry to reg.robotunnel.io (VPS, port 8090, Caddy, systemd). Use when asked to deploy/release the registry, push a new registry build to production, or restart the reg.robotunnel.io service.
---

# deploy-registry

Deploy the registry to `reg.robotunnel.io`. The service runs on the shared VPS at
port 8090 behind Caddy, as systemd unit `robot-agent-registry`, with its own
Postgres database (separate from Operations). Migrations apply automatically on
startup. Full details: `deploy/README.md`.

## Preferred: GitHub Action

Trigger the **Deploy Registry** workflow (`.github/workflows/deploy-registry.yml`,
`workflow_dispatch`), choosing `goarch` to match the VPS (`amd64` by default). It
builds, uploads, and runs `setup.sh`. It never overwrites `.env`.

```bash
gh workflow run "Deploy Registry" -f goarch=amd64
gh run watch
```

## Manual deploy

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o robot-agent-registry ./cmd/registry
scp robot-agent-registry deploy/registry.service deploy/setup.sh root@<vps>:/tmp/
ssh root@<vps> 'cd /tmp && ./setup.sh'
```

## First-time only (needs human-provided infra)

Not automatable — confirm these exist before a first deploy:
- DNS `reg.robotunnel.io → <VPS IP>`.
- A dedicated Postgres (e.g. a new Supabase project); put its URL + a fresh
  `JWT_SIGNING_KEY` (`openssl rand -hex 32`) in `/opt/robot-agent-registry/config/.env`.
- Run `deploy/bootstrap.sh` on the VPS once (creates user, `.env` template, Caddy vhost).

## Verify after deploy

```bash
curl -s https://reg.robotunnel.io/v1/discover/agents      # → {"agents":[],"total":0,...}
ssh root@<vps> 'systemctl status robot-agent-registry'    # active (running)
```

Do not deploy without confirming the target and that prerequisites are in place;
this is a production, outward-facing action.
