#!/usr/bin/env bash
set -euo pipefail

VPS_HOST="31.76.76.12"
VPS_USER="root"
APP_DIR="/opt/saveinator"
REPO="git@github.com:pyfig/saveinator.git"
BRANCH="main"

echo "=== Deploying Saveinator to $VPS_HOST ==="

echo "[1/6] Syncing code to VPS..."
ssh "$VPS_USER@$VPS_HOST" "
    if [ -d '$APP_DIR' ]; then
        cd '$APP_DIR' && git fetch origin && git reset --hard origin/$BRANCH
    else
        git clone '$REPO' '$APP_DIR' && cd '$APP_DIR'
    fi
"

echo "[2/6] Setting up .env..."
ssh "$VPS_USER@$VPS_HOST" "
    cd '$APP_DIR'
    if [ ! -f .env ]; then
        cp .env.example .env
        echo '=== EDIT /opt/saveinator/.env with real values before starting ==='
    fi
"

echo "[3/6] Installing certbot and obtaining TLS certificate..."
ssh "$VPS_USER@$VPS_HOST" "
    if ! command -v certbot > /dev/null 2>&1; then
        apt-get update && apt-get install -y certbot python3-certbot-nginx
    fi
    certbot --nginx -d xdshka.party --non-interactive --agree-tos -m admin@xdshka.party || true
"

echo "[4/6] Pulling Docker images and starting services..."
ssh "$VPS_USER@$VPS_HOST" "
    cd '$APP_DIR'
    docker compose pull
    docker compose build
    docker compose up -d
"

echo "[5/6] Running database migrations..."
ssh "$VPS_USER@$VPS_HOST" "
    cd '$APP_DIR'
    docker compose exec -T webhook alembic upgrade head || echo 'Migrations may need manual run'
"

echo "[6/6] Installing systemd service..."
ssh "$VPS_USER@$VPS_HOST" "
    cp '$APP_DIR/systemd/ytbot.service' /etc/systemd/system/ytbot.service
    systemctl daemon-reload
    systemctl enable ytbot
"

echo "=== Deployment complete! ==="
echo "Check status: ssh $VPS_USER@$VPS_HOST 'systemctl status ytbot'"
echo "Check logs:  ssh $VPS_USER@$VPS_HOST 'docker compose -f $APP_DIR/docker-compose.yml logs -f'"
