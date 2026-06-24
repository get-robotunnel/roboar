#!/usr/bin/env bash
# Install/refresh the registry binary + systemd unit on the VPS.
# Expects ./robot-agent-registry (the linux binary) and ./registry.service in CWD.
# Used by the Deploy Registry GitHub workflow; safe to run by hand as root.
# Does NOT touch /opt/robot-agent-registry/config/.env.
set -euo pipefail

APP_DIR=/opt/robot-agent-registry
BIN_DIR="$APP_DIR/bin"

id -u robotunnel >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin robotunnel || true
install -d -o robotunnel -g robotunnel "$APP_DIR" "$BIN_DIR" "$APP_DIR/config"

install -m 0755 ./robot-agent-registry "$BIN_DIR/robot-agent-registry"
install -m 0644 ./registry.service /etc/systemd/system/robot-agent-registry.service

systemctl daemon-reload
systemctl enable robot-agent-registry
systemctl restart robot-agent-registry
echo "robot-agent-registry restarted; status:"
systemctl --no-pager --full status robot-agent-registry | head -n 8 || true
