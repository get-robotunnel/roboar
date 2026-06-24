#!/usr/bin/env bash
# One-time VPS preparation for the registry: config dir, .env template, Caddy
# vhost. Run as root. Re-running is safe; it will not overwrite an existing .env.
set -euo pipefail

APP_DIR=/opt/robot-agent-registry
CONF="$APP_DIR/config/.env"

id -u robotunnel >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin robotunnel
install -d -o robotunnel -g robotunnel "$APP_DIR" "$APP_DIR/bin" "$APP_DIR/config"

if [ ! -f "$CONF" ]; then
  cat >"$CONF" <<EOF
PORT=8090
# Postgres connection string for the registry's OWN database (separate from Operations).
DATABASE_URL=postgres://USER:PASSWORD@HOST:5432/postgres?sslmode=require
# HS256 signing key for owner JWTs — generate with: openssl rand -hex 32
JWT_SIGNING_KEY=CHANGE_ME
REGISTRY_BASE_URL=https://reg.robotunnel.io/v1
HEARTBEAT_OFFLINE_SECS=60
EOF
  chmod 600 "$CONF"
  chown robotunnel:robotunnel "$CONF"
  echo "Wrote template $CONF — edit DATABASE_URL and JWT_SIGNING_KEY before starting."
else
  echo "$CONF already exists; leaving it untouched."
fi

# Append the Caddy vhost if not already present.
CADDYFILE=${CADDYFILE:-/etc/caddy/Caddyfile}
if [ -f "$CADDYFILE" ] && ! grep -q "reg.robotunnel.io" "$CADDYFILE"; then
  printf '\nreg.robotunnel.io {\n\treverse_proxy 127.0.0.1:8090\n}\n' >>"$CADDYFILE"
  systemctl reload caddy || true
  echo "Added reg.robotunnel.io vhost to $CADDYFILE and reloaded Caddy."
fi

echo "Bootstrap done. Next: run setup.sh to install the binary, then 'systemctl start robot-agent-registry'."
