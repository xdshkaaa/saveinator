## 1. Sender: HTML-aware edit

- [x] 1.1 Add `EditMessageHTML(chatID int64, messageID int, text string) error` to `go/internal/sender/telegram.go`, mirroring `EditMessage` but with `ParseMode: telego.ModeHTML`.
- [x] 1.2 Add `EditMessageHTML(chatID int64, messageID int, text string) error` to the sender interface in `go/internal/worker/deps.go`.

## 2. Worker: use HTML edit for status string

- [x] 2.1 Replace `EditMessage` with `EditMessageHTML` at the `download.downloading` edit site in `worker/download.go:195` (runDownload).
- [x] 2.2 Replace `EditMessage` with `EditMessageHTML` at the `download.downloading` edit site in `worker/download.go:397` (runPinterest).
- [x] 2.3 Replace `EditMessage` with `EditMessageHTML` at the `download.downloading` edit site in `worker/tiktok.go:26`.
- [x] 2.4 Confirm no other worker `EditMessage` call targets a `<tg-emoji>` locale string (grep locales for `tg-emoji`; only `download.downloading` is worker-edited).

## 3. Tests & verification

- [x] 3.1 Update the recording fake sender in `worker/download_test.go` to implement `EditMessageHTML` and record which edit method was used.
- [x] 3.2 Add/extend a test asserting the download-status edit goes through the HTML edit path (not plain `EditMessage`), and error-string edits still use plain `EditMessage`.
- [x] 3.3 Run `cd go && go build ./... && go test -race -count=1 ./internal/sender/... ./internal/worker/...`.
