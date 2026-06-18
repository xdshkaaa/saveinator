#!/bin/bash
set -euo pipefail
cd /opt/saveinator
git fetch origin && git reset --hard origin/main
docker compose up -d --force-recreate worker
sleep 3
w=$(docker compose ps -q worker)
for f in workers/downloader.py workers/youtube_format.py workers/video_processor.py workers/tasks.py bot/services/file_sender.py; do
  docker cp "$f" "$w:/app/$f"
done
docker compose restart worker
sleep 3
docker compose exec -T worker python -c "from workers.youtube_format import build_youtube_format; print(build_youtube_format(720))"
docker compose ps worker
