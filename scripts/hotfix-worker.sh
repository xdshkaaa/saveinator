#!/usr/bin/env bash
# Apply worker code changes without rebuilding the image (safe on low-RAM VPS).
set -euo pipefail

VPS_HOST="${VPS_HOST:-103.214.69.38}"
VPS_USER="${VPS_USER:-root}"
APP_DIR="${APP_DIR:-/opt/saveinator}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=== Hotfix worker on $VPS_HOST (no docker build) ==="
tr -d '\r' < "$SCRIPT_DIR/apply-on-vps.sh" | ssh "$VPS_USER@$VPS_HOST" bash
echo "=== Hotfix complete ==="
