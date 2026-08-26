#!/usr/bin/env bash
# Deploy the operator dashboard (dash) to the VPS: builds the dash container,
# wires up Caddy (:8094, basic auth) and the Cloudflare Tunnel ingress, and
# verifies the public hostname.
#
# Unlike the full deploy (deploy.sh), this script PATCHES the live Caddyfile
# and cloudflared config on the VPS instead of replacing them, because those
# files are shared with other projects on the same host. Both are backed up
# before any change.
set -euo pipefail

VPS_HOST="${VPS_HOST:?set VPS_HOST env var}"
VPS_USER="root"
APP_DIR="/opt/saveinator"
DASH_USER="${DASH_AUTH_USER:?set DASH_AUTH_USER env var}"
DASH_PASS="${DASH_AUTH_PASSWORD:?set DASH_AUTH_PASSWORD env var}"

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

echo "[3/6] Patching Caddy (block :8098 + basic auth)..."
# The repo file monitoring/caddy-grafana.caddyfile contains the exact block
# to insert; pull it out and splice it into the live Caddyfile if missing.
# The marker comment is what makes the block unique — :PORT numbers on this
# host are shared with other projects, so we match on the dash comment.
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
        echo 'Caddy: dash block already present, skipping'
    fi
"

echo "[4/6] Setting up basic auth..."
ssh "$VPS_USER@$VPS_HOST" "
    set -euo pipefail
    HASH=\$(caddy hash-password --plaintext '$DASH_PASS')
    printf '%s:%s\n' '$DASH_USER' \"\$HASH\" > /etc/caddy/dash.htpasswd
    chmod 600 /etc/caddy/dash.htpasswd
    echo 'dash.htpasswd written'
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
    echo '--- caddy no auth (expect 401) ---'
    curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8098/
    echo '--- caddy with auth (expect 200) ---'
    curl -s -o /dev/null -w '%{http_code}\n' -u '$DASH_USER:$DASH_PASS' http://127.0.0.1:8098/
    echo '--- public hostname ---'
    curl -s -o /dev/null -w '%{http_code}\n' -u '$DASH_USER:$DASH_PASS' https://dash-saveinator.xdshka.party/
"

echo ""
echo "=== Done! ==="
echo "If the public check above did not return 200:"
echo "  1. dig +short dash-saveinator.xdshka.party"
echo "  2. If empty, add a CNAME dash-saveinator -> <tunnel-id>.cfargotunnel.com in Cloudflare DNS"
echo "     (tunnel id is the first line of /etc/cloudflared/config.yml)"
echo "  3. If a wildcard *.xdshka.party CNAME already exists, it should just work."
