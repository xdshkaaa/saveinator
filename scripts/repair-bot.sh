#!/usr/bin/env bash
set -euo pipefail

VPS_HOST="${VPS_HOST:-103.214.69.38}"
VPS_USER="${VPS_USER:-root}"
APP_DIR="${APP_DIR:-/opt/saveinator}"

echo "=== Repairing Saveinator on $VPS_HOST ==="

ssh "$VPS_USER@$VPS_HOST" "
    set -euo pipefail
    cd '$APP_DIR'

    bot_len=\$(docker compose exec -T bot python3 -c 'import os; print(len(os.environ.get(\"BOT_TOKEN\", \"\")))')
    worker_len=\$(docker compose exec -T worker python3 -c 'import os; print(len(os.environ.get(\"BOT_TOKEN\", \"\")))')
    file_len=\$(awk -F= '/^BOT_TOKEN=/ {print length(\$2)}' .env | tr -d ' \"'\''')

    echo \"token lens: file=\$file_len bot=\$bot_len worker=\$worker_len\"

    if [[ \"\$bot_len\" != \"\$file_len\" || \"\$worker_len\" != \"\$file_len\" ]]; then
        echo 'Recreating bot and worker to reload BOT_TOKEN from .env...'
        docker compose up -d --force-recreate bot worker
        sleep 5
    fi

    docker compose ps bot worker
    docker compose logs --tail=8 bot
    docker compose logs --tail=8 worker
"

echo "=== Repair complete ==="
