#!/usr/bin/env bash
set -euo pipefail

VPS_HOST="${VPS_HOST:?set VPS_HOST env var}"
VPS_USER="${VPS_USER:-root}"
APP_DIR="${APP_DIR:-/opt/saveinator}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COOKIES_FILE="$ROOT/secrets/tiktok_cookies.txt"

"$ROOT/scripts/export_tiktok_cookies.sh" "$COOKIES_FILE"

ssh "$VPS_USER@$VPS_HOST" "mkdir -p $APP_DIR/secrets"
scp "$COOKIES_FILE" "$VPS_USER@$VPS_HOST:$APP_DIR/secrets/tiktok_cookies.txt"

ssh "$VPS_USER@$VPS_HOST" "grep -q '^TIKTOK_COOKIES_PATH=' $APP_DIR/.env 2>/dev/null && \
  sed -i 's|^TIKTOK_COOKIES_PATH=.*|TIKTOK_COOKIES_PATH=/secrets/tiktok_cookies.txt|' $APP_DIR/.env || \
  printf '\nTIKTOK_COOKIES_PATH=/secrets/tiktok_cookies.txt\nTIKTOK_COOKIES_FROM_BROWSER=\n' >> $APP_DIR/.env"

ssh "$VPS_USER@$VPS_HOST" "cd $APP_DIR && docker compose up -d --force-recreate saveinator"

echo "TikTok cookies deployed to $VPS_HOST"
