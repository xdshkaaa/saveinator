#!/bin/bash
set -euo pipefail
cd /opt/saveinator
git fetch origin && git reset --hard origin/main
docker compose build spotify
docker compose up -d --force-recreate spotify
sleep 5
curl -fsS http://127.0.0.1:8003/health
curl -fsS http://127.0.0.1:9105/metrics | head -5
docker compose ps spotify
