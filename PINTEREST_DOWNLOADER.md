# Pinterest Downloader

Pinterest support is implemented in Go. See:

- Skill: `.cursor/skills/saveinator-pinterest/SKILL.md`
- Parser: `go/internal/pinterest/parser.go`
- API client: `go/internal/pinterest/client.go`
- Worker: `go/internal/worker/pinterest_tiktok.go`
- HTTP API: `go/internal/api/pinterest.go`

Pins and short links use Pinterest `PinResource` API. Boards use `BoardFeedResource` API (not `pinterest-dl`).
