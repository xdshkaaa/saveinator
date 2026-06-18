#!/bin/bash
set -euo pipefail
cd /opt/saveinator
docker cp scripts/test_ytdlp_youtube.py "$(docker compose ps -q worker):/tmp/test_ytdlp_youtube.py"
docker compose exec -T worker python /tmp/test_ytdlp_youtube.py https://youtu.be/jNQXAC9IVRw 480
echo "--- recent worker logs ---"
docker compose logs --tail=25 worker
