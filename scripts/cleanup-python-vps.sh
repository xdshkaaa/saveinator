#!/usr/bin/env bash
# Remove legacy Python Docker artifacts from VPS after Go cutover.
set -euo pipefail

VPS_HOST="${VPS_HOST:-103.214.69.38}"
VPS_USER="${VPS_USER:-root}"
APP_DIR="${APP_DIR:-/opt/saveinator}"

echo "=== Cleaning legacy Python artifacts on $VPS_HOST ==="
ssh "$VPS_USER@$VPS_HOST" "APP_DIR='$APP_DIR' bash -s" < "$(dirname "$0")/cleanup-python-vps-remote.sh"
echo "=== Cleanup complete ==="
