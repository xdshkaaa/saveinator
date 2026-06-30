---
name: saveinator-deploy-vps
description: >-
  Deploys and repairs Saveinator on the production VPS. Use when deploying, hotfixing,
  repairing BOT_TOKEN mismatch, VPS production, cutover, or docker compose on /opt/saveinator.
---

# Saveinator VPS Deploy

Production VPS defaults (override via env):

| Variable | Default |
|----------|---------|
| `VPS_HOST` | `103.214.69.38` |
| `VPS_USER` | `root` |
| `APP_DIR` | `/opt/saveinator` |
| Branch | `main` |

## Full deploy

```bash
./scripts/deploy.sh
```

Steps:
1. Ensure VPS GitHub deploy key (interactive on first run)
2. `git fetch && git reset --hard origin/main` in `/opt/saveinator`
3. Bootstrap `.env` from `.env.example` if missing
4. `docker compose build saveinator && docker compose up -d --force-recreate saveinator`
5. `docker compose --profile tools run --rm migrate`
6. Install `systemd/ytbot.service` → `ytbot.service` (legacy name; runs compose in `/opt/saveinator`)

## Hotfix (code sync + rebuild)

```bash
./scripts/hotfix-worker.sh
```

Pipes `scripts/apply-on-vps.sh` over SSH:
- `git fetch && git reset --hard origin/main`
- rebuild + force-recreate `saveinator`
- verify `curl :8000/health` and `:9101/metrics`

## Repair BOT_TOKEN mismatch

```bash
./scripts/repair-bot.sh
```

Compares `.env` `BOT_TOKEN` length vs container env; force-recreates `saveinator` if mismatched.

## Verify after deploy

```bash
ssh root@103.214.69.38 'curl -fsS http://127.0.0.1:8000/health'
ssh root@103.214.69.38 'curl -fsS http://127.0.0.1:9101/metrics | head'
ssh root@103.214.69.38 'curl -fsS http://127.0.0.1:9102/metrics | head'
ssh root@103.214.69.38 'docker compose -f /opt/saveinator/docker-compose.yml ps'
```

Public endpoints:
- Webhook: `https://saveinator-hooks.xdshka.party/webhook`
- Grafana: `https://saveinator.xdshka.party` (see `saveinator-monitoring`)

## Historical one-shots (post-migration only)

| Script | Purpose |
|--------|---------|
| `scripts/cutover-to-go.sh` | pg_dump backup, stop legacy `bot`/`worker`, switch to Go `saveinator` |
| `scripts/cleanup-python-vps.sh` | Remove old Python Docker images |
| `scripts/cleanup-python-vps-remote.sh` | Remote variant of cleanup |

Do not run cutover/cleanup on a Go-only VPS unless reverting or cleaning legacy artifacts.

## Related

- Schema changes: `saveinator-db-migrate`
- Cookie deploy: `saveinator-cookies`
- Local dev: `saveinator-go-dev`
