#!/bin/bash
set -euo pipefail
cd /opt/saveinator
git fetch origin && git reset --hard origin/main
docker compose build saveinator
docker compose up -d --force-recreate saveinator
sleep 5
curl -fsS http://127.0.0.1:8000/health
curl -fsS http://127.0.0.1:9101/metrics | head -5
docker compose ps saveinator
