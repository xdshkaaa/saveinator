#!/usr/bin/env bash
# Rebuild and restart the Go saveinator container on VPS.
set -euo pipefail

VPS_HOST="${VPS_HOST:-103.214.69.38}"
VPS_USER="${VPS_USER:-root}"
APP_DIR="${APP_DIR:-/opt/saveinator}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=== Hotfix saveinator on $VPS_HOST ==="
tr -d '\r' < "$SCRIPT_DIR/apply-on-vps.sh" | ssh "$VPS_USER@$VPS_HOST" bash
echo "=== Hotfix complete ==="
