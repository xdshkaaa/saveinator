#!/usr/bin/env bash
# Deploy the operator dashboard (dash) to the VPS: builds the dash container,
# wires up Caddy (:8098) and the Cloudflare Tunnel ingress, and verifies the
# public hostname.
#
# Auth is inside the app (Telegram Login), so no basic-auth credentials are
# needed at the proxy. The bot used by the Login Widget must have the domain
# whitelisted: @BotFather → /setdomain → dash-saveinator.xdshka.party.
#
# Unlike the full deploy (deploy.sh), this script PATCHES the live Caddyfile
# and cloudflared config on the VPS instead of replacing them, because those
# files are shared with other projects on the same host. Both are backed up
# before any change.
set -euo pipefail

VPS_HOST="${VPS_HOST:?set VPS_HOST env var}"
VPS_USER="root"
APP_DIR="/opt/saveinator"
# Optional: which Telegram bot powers the Login Widget and who may log in.
# Defaults match the compose service (main bot token + owner id).
DASH_TELEGRAM_TOKEN="${DASH_TELEGRAM_TOKEN:-}"
DASH_ADMIN_IDS="${DASH_ADMIN_IDS:-339193247}"

echo "=== Deploying dash to $VPS_HOST ==="

echo "[1/6] Syncing code to VPS..."
ssh "$VPS_USER@$VPS_HOST" "
    cd '$APP_DIR'
    git fetch origin && git reset --hard origin/main
"

echo "[2/6] Building and starting dash container..."
ssh "$VPS_USER@$VPS_HOST" "
    cd '$APP_DIR'
    docker compose build dash
    docker compose up -d dash
    for i in \$(seq 1 30); do
        curl -fsS http://127.0.0.1:9000/api/health && break
        sleep 1
    done
"

echo "[3/6] Patching Caddy (block :8098)..."
# The repo file monitoring/caddy-grafana.caddyfile contains the exact block
# to insert; pull it out and splice it into the live Caddyfile if missing.
# The marker comment is what makes the block unique — :PORT numbers on this
# host are shared with other projects, so we match on the dash comment.
# When the block already exists, it is REPLACED wholesale from the repo file
# (older blocks carried basic_auth and must not survive).
ssh "$VPS_USER@$VPS_HOST" "
    set -euo pipefail
    MARK='# dash-saveinator.xdshka.party'
    BLOCK=\$(awk '/^:8098 \{/,/^\}/' '$APP_DIR/monitoring/caddy-grafana.caddyfile')
    if [ -z \"\$BLOCK\" ]; then
        echo 'ERROR: :8098 block not found in repo caddyfile' >&2
        exit 1
    fi
    if ! grep -qF \"\$MARK\" /etc/caddy/Caddyfile; then
        cp /etc/caddy/Caddyfile /etc/caddy/Caddyfile.bak.\$(date +%Y%m%d%H%M%S)
        printf '\n%s\n' \"\$BLOCK\" >> /etc/caddy/Caddyfile
        echo 'Caddy: block :8098 appended (backup created)'
    else
        cp /etc/caddy/Caddyfile /etc/caddy/Caddyfile.bak.\$(date +%Y%m%d%H%M%S)
        # Replace the old :8098 block between its braces with the repo block,
        # preserving the rest of the file. python3 is present on the VPS.
        python3 -c '
import re, sys
src = open(\"/etc/caddy/Caddyfile\").read()
block = open(\"'\"$APP_DIR\"'/monitoring/caddy-grafana.caddyfile\").read()
m = re.search(r\":8098 \\{.*?\\n\\}\", src, re.S)
if not m:
    sys.exit(\"ERROR: :8098 block not found in live Caddyfile\")
new = re.sub(r\":8098 \\{.*?\\n\\}\", block.rstrip() + \"\\n\", src, count=1, flags=re.S)
open(\"/etc/caddy/Caddyfile\", \"w\").write(new)
'
        echo 'Caddy: :8098 block replaced from repo (backup created)'
    fi
"

echo "[4/6] Ensuring access log ownership..."
ssh "$VPS_USER@$VPS_HOST" "
    set -euo pipefail
    # pre-create the access log with caddy ownership (caddy can't open root-owned files).
    touch /var/log/caddy/dash-saveinator.xdshka.party.access.log
    chown caddy:caddy /var/log/caddy/dash-saveinator.xdshka.party.access.log
    echo 'access log ready'
"

echo "[5/6] Patching cloudflared ingress..."
ssh "$VPS_USER@$VPS_HOST" "
    set -euo pipefail
    CONF=/etc/cloudflared/config.yml
    if ! grep -q 'dash-saveinator.xdshka.party' \$CONF; then
        cp \$CONF \$CONF.bak.\$(date +%Y%m%d%H%M%S)
        # insert the dash hostname entry right before the first saveinator entry
        awk '/hostname: saveinator.xdshka.party/ && !done {
            print \"  - hostname: dash-saveinator.xdshka.party\"
            print \"    service: http://localhost:8098\"
            done=1
        } { print }' \$CONF > \$CONF.new && mv \$CONF.new \$CONF
        echo 'cloudflared: dash ingress added (backup created)'
    else
        echo 'cloudflared: dash ingress already present, skipping'
    fi
"

echo "[6/6] Reloading proxies and verifying..."
ssh "$VPS_USER@$VPS_HOST" "
    set -euo pipefail
    caddy validate --config /etc/caddy/Caddyfile >/dev/null && systemctl reload caddy
    systemctl restart cloudflared
    sleep 3
    echo '--- dash direct (expect 200) ---'
    curl -fsS http://127.0.0.1:9000/api/health
    echo
    echo '--- auth status (expect authed:false) ---'
    curl -fsS http://127.0.0.1:9000/api/auth/status
    echo
    echo '--- overview without session (expect 401) ---'
    curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:9000/api/overview
    echo '--- public hostname (expect 200) ---'
    curl -s -o /dev/null -w '%{http_code}\n' https://dash-saveinator.xdshka.party/
"

echo ""
echo "=== Done! ==="
echo "The app now authenticates via Telegram Login Widget."
echo "Remember in @BotFather: /setdomain for the widget bot -> dash-saveinator.xdshka.party"
echo "If the public check above did not return 200:"
echo "  1. dig +short dash-saveinator.xdshka.party"
echo "  2. If empty, add a CNAME dash-saveinator -> <tunnel-id>.cfargotunnel.com in Cloudflare DNS"
echo "     (tunnel id is the first line of /etc/cloudflared/config.yml)"
echo "  3. If a wildcard *.xdshka.party CNAME already exists, it should just work."
