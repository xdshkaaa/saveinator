#!/usr/bin/env bash
# Apply worker code changes without rebuilding the image (safe on low-RAM VPS).
set -euo pipefail

VPS_HOST="${VPS_HOST:-103.214.69.38}"
VPS_USER="${VPS_USER:-root}"
APP_DIR="${APP_DIR:-/opt/saveinator}"

echo "=== Hotfix worker on $VPS_HOST (no docker build) ==="

ssh "$VPS_USER@$VPS_HOST "
    set -euo pipefail
    cd '$APP_DIR'
    git fetch origin && git reset --hard origin/main
    worker=\$(docker compose ps -q worker)
    if [ -z \"\$worker\" ]; then
        echo 'Worker container not running; starting stack...'
        docker compose up -d worker
        exit 0
    fi
    for f in workers/downloader.py workers/youtube_format.py workers/video_processor.py workers/tasks.py; do
        docker cp \"\$f\" \"\$worker:/app/\$f\"
    done
    docker compose up -d --force-recreate worker
    docker compose ps worker
    docker compose logs --tail=5 worker
"

echo "=== Hotfix complete ==="
