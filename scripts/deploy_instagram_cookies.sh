#!/usr/bin/env bash
set -euo pipefail

VPS_HOST="${VPS_HOST:-103.214.69.38}"
VPS_USER="${VPS_USER:-root}"
APP_DIR="${APP_DIR:-/opt/saveinator}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COOKIES_FILE="$ROOT/secrets/instagram_cookies.txt"

"$ROOT/scripts/export_instagram_cookies.sh" "$COOKIES_FILE"

ssh "$VPS_USER@$VPS_HOST" "mkdir -p $APP_DIR/secrets"
scp "$COOKIES_FILE" "$VPS_USER@$VPS_HOST:$APP_DIR/secrets/instagram_cookies.txt"

ssh "$VPS_USER@$VPS_HOST" "grep -q '^INSTAGRAM_COOKIES_PATH=' $APP_DIR/.env 2>/dev/null && \
  sed -i 's|^INSTAGRAM_COOKIES_PATH=.*|INSTAGRAM_COOKIES_PATH=/secrets/instagram_cookies.txt|' $APP_DIR/.env || \
  printf '\nINSTAGRAM_COOKIES_PATH=/secrets/instagram_cookies.txt\nINSTAGRAM_COOKIES_FROM_BROWSER=\n' >> $APP_DIR/.env"

ssh "$VPS_USER@$VPS_HOST" "cd $APP_DIR && docker compose up -d --force-recreate saveinator"

echo "Instagram cookies deployed to $VPS_HOST"
