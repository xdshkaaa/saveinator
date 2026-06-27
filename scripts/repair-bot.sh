#!/usr/bin/env bash
set -euo pipefail

VPS_HOST="${VPS_HOST:-103.214.69.38}"
VPS_USER="${VPS_USER:-root}"
APP_DIR="${APP_DIR:-/opt/saveinator}"

echo "=== Repairing Saveinator on $VPS_HOST ==="

ssh "$VPS_USER@$VPS_HOST" "
    set -euo pipefail
    cd '$APP_DIR'

    file_len=\$(awk -F= '/^BOT_TOKEN=/ {print length(\$2)}' .env | tr -d ' \"'\''')
    svc_len=\$(docker compose exec -T saveinator sh -c 'printf %s \"\$BOT_TOKEN\" | wc -c' | tr -d ' ')

    echo \"token lens: file=\$file_len saveinator=\$svc_len\"

    if [[ \"\$svc_len\" != \"\$file_len\" ]]; then
        echo 'Recreating saveinator to reload BOT_TOKEN from .env...'
        docker compose up -d --force-recreate saveinator
        sleep 5
    fi

    docker compose ps saveinator db redis
    docker compose logs --tail=20 saveinator
"

echo "=== Repair complete ==="
