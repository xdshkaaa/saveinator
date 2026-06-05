#!/usr/bin/env bash
set -euo pipefail

VPS_HOST="31.76.76.12"
VPS_USER="root"
APP_DIR="/opt/saveinator"
REPO="git@github.com:pyfig/saveinator.git"
BRANCH="main"

echo "=== Deploying Saveinator to $VPS_HOST ==="

echo "[0/6] Ensuring VPS can access GitHub via SSH..."
ssh "$VPS_USER@$VPS_HOST" "
    ssh-keyscan -H github.com >> ~/.ssh/known_hosts 2>/dev/null

    if [ ! -f ~/.ssh/id_ed25519 ]; then
        ssh-keygen -t ed25519 -N '' -f ~/.ssh/id_ed25519 -C 'saveinator-vps'
    fi

    if ! ssh -o StrictHostKeyChecking=accept-new -T git@github.com 2>&1 | grep -q successfully; then
        echo ''
        echo '=== ADD THIS KEY TO GITHUB DEPLOY KEYS ==='
        echo 'URL: https://github.com/pyfig/saveinator/settings/keys'
        echo ''
        cat ~/.ssh/id_ed25519.pub
        echo ''
        echo 'Press Enter after adding the key...'
        read
    fi
"

echo "[1/6] Syncing code to VPS..."
ssh "$VPS_USER@$VPS_HOST" "
    if [ -d '$APP_DIR' ]; then
        cd '$APP_DIR' && git fetch origin && git reset --hard origin/$BRANCH
    else
        git clone '$REPO' '$APP_DIR'
    fi
"

echo "[2/6] Setting up .env..."
ssh "$VPS_USER@$VPS_HOST" "
    cd '$APP_DIR'
    if [ ! -f .env ]; then
        cp .env.example .env
        echo '=== >>> WARNING: Edit /opt/saveinator/.env with real values BEFORE starting! <<<'
    fi
"

echo "[3/6] Pulling Docker images and starting services..."
ssh "$VPS_USER@$VPS_HOST" "
    cd '$APP_DIR'
    docker compose pull
    docker compose build
    docker compose up -d
"

echo "[4/6] Running database migrations..."
ssh "$VPS_USER@$VPS_HOST" "
    cd '$APP_DIR'
    docker compose exec -T bot alembic upgrade head || echo '(migrations deferred — run manually after .env is set)'
"

echo "[5/6] Installing systemd service..."
ssh "$VPS_USER@$VPS_HOST" "
    cp '$APP_DIR/systemd/ytbot.service' /etc/systemd/system/ytbot.service
    systemctl daemon-reload
    systemctl enable ytbot
"

echo ""
echo "=== Deployment complete! ==="
echo "Check status: ssh $VPS_USER@$VPS_HOST 'systemctl status ytbot'"
echo "Check logs:  ssh $VPS_USER@$VPS_HOST 'docker compose -f $APP_DIR/docker-compose.yml logs -f'"
