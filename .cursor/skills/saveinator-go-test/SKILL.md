---
name: saveinator-go-test
description: >-
  Runs and writes Saveinator Go tests before PR or after Go changes. Use when running go test,
  adding Go unit tests, fixing CI job go, or verifying go/ compiles.
---

# Saveinator Go Test Loop

CI (`.github/workflows/ci.yml`) runs three jobs:

| Job | What it checks |
|-----|----------------|
| `go` | Go **1.22.3**: `go build ./...`, `go test -race -count=1 -coverprofile=coverage.out ./...` |
| `migrate-image` | `docker build -f docker/migrate/Dockerfile .` |
| `compose` | `docker compose config -q` |

## Commands

```bash
# Full suite (matches CI go job)
cd go && go build ./... && go test -race -count=1 -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# Verbose / targeted
cd go && go test -v ./internal/linkparser/...
cd go && go test -v ./internal/handler/...
cd go && go test -run TestName ./internal/...
```

## When to run

| Change | Minimum tests |
|--------|---------------|
| `linkparser/` | `go test ./internal/linkparser/...` |
| `handler/` | `go test ./internal/handler/...` |
| `queue/`, `worker/` | `go test ./...` (full) |
| `config/` | `go test ./internal/config/...` |
| `db/migrations/`, `docker-compose.yml` | Also run migrate-image + compose CI jobs locally |
| Any `go/` change | `go build ./... && go test ./...` before claiming done |

## Conventions

- **Table-driven tests** with `t.Parallel()` where safe — see `linkparser/parser_test.go`, `handler/routing_test.go`
- **Real URL fixtures** in linkparser tests — keep cases comprehensive in `parser_test.go`
- **Mock-free unit tests** in Go; DB integration uses `go/internal/db/testdata/schema.sql`
- No test database required for most Go packages (pure parsing/routing)

## Existing Go test files

```
go/internal/config/config_test.go
go/internal/db/enum_test.go
go/internal/linkparser/parser_test.go
go/internal/pinterest/parser_test.go
go/internal/queue/clear_test.go
go/internal/sender/telegram_test.go
go/internal/tiktok/downloader_test.go
go/internal/xphotos/downloader_test.go
go/internal/youtube/keyboards_test.go
go/internal/ytdlp/downloader_test.go
go/internal/ytdlp/errors_test.go
```

Add tests alongside the package you change; prefer extending existing `*_test.go` files.

## Pre-PR checklist

```
- [ ] cd go && go build ./...
- [ ] cd go && go test -race -count=1 ./...
- [ ] scripts/check-parity.sh (if locales changed)
- [ ] docker build -f docker/migrate/Dockerfile . (if schema/migrate Dockerfile changed)
- [ ] docker compose config -q (if compose files changed)
```

## CI failure debug

1. Reproduce locally: `cd go && go test -v ./...`
2. Check Go version: `go version` (should be 1.22+)
3. Module issues: `cd go && go mod tidy && go test ./...`
