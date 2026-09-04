package worker

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"saveinator/internal/cookies"
	"saveinator/internal/db"
	"saveinator/internal/pinterest"
	"saveinator/internal/reddit"
	"saveinator/internal/xphotos"
	"saveinator/internal/youtube"
	"saveinator/internal/ytdlp"
)

const (
	testRunnerInterval  = 15 * time.Second
	testDefaultTimeout  = 5 * time.Minute
	testYouTubeQuality  = 720
	testMaxErrorMessage = 500
)

// StartTestRunner launches the background loop that walks the test_urls
// checklist (dash «Тест ссылок»): each URL is run through its platform
// scenario into a throwaway temp dir, nothing is sent to Telegram and no
// downloads row is recorded. URLs are processed strictly one at a time.
func (h *Handler) StartTestRunner(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(testRunnerInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.drainTestURLs(ctx)
			}
		}
	}()
}

// drainTestURLs claims and runs queued rows until the checklist is empty.
func (h *Handler) drainTestURLs(ctx context.Context) {
	for {
		row, err := h.db.ClaimNextTestURL(ctx)
		if err != nil {
			slog.Warn("test runner claim failed", "err", err)
			return
		}
		if row == nil {
			return
		}
		h.runTestURL(ctx, row)
	}
}

func (h *Handler) runTestURL(ctx context.Context, row *db.TestURLRow) {
	start := time.Now()
	slog.Info("test url started", "id", row.ID, "platform", row.Platform, "url", row.URL)

	status, errMsg, mediaType, size := h.executeTestURL(ctx, row)

	durMS := int(time.Since(start).Milliseconds())
	if err := h.db.FinishTestURL(ctx, row.ID, status, errMsg, mediaType, size, durMS); err != nil {
		slog.Warn("test runner finish failed", "id", row.ID, "err", err)
	}
	if status == db.TestStatusPassed {
		slog.Info("test url passed", "id", row.ID, "platform", row.Platform,
			"media", mediaType, "size", size, "duration_ms", durMS)
	} else {
		slog.Warn("test url failed", "id", row.ID, "platform", row.Platform,
			"err", errMsg, "duration_ms", durMS)
	}
}

// executeTestURL dispatches the platform scenario; PASS means real media
// files landed in the temp dir.
func (h *Handler) executeTestURL(ctx context.Context, row *db.TestURLRow) (status, errMsg, mediaType string, size int64) {
	taskDir, err := os.MkdirTemp("", "saveinator-test-*")
	if err != nil {
		return db.TestStatusFailed, "temp dir: " + err.Error(), "", 0
	}
	defer os.RemoveAll(taskDir)

	timeout := testDefaultTimeout
	if sec := h.runtime.PlatformTimeoutSec(ctx, row.Platform); sec > 0 {
		timeout = time.Duration(sec) * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch row.Platform {
	case "youtube", "tiktok", "instagram":
		return h.testYtdlpPlatform(runCtx, row.URL, row.Platform, taskDir)
	case "x":
		return h.testX(runCtx, row.URL, taskDir)
	case "pinterest":
		return h.testPinterest(runCtx, row.URL, taskDir)
	case "reddit":
		return h.testReddit(runCtx, row.URL, taskDir)
	default:
		return db.TestStatusFailed, "platform not testable: " + row.Platform, "", 0
	}
}

// testYtdlpPlatform mirrors the generic runDownload scenario (minus the
// Telegram send): ytdlp.Download with production opts, then media check.
// YouTube goes through the same format cascade as production downloads —
// a bare "best" fails on videos where YouTube no longer publishes
// progressive renditions.
func (h *Handler) testYtdlpPlatform(ctx context.Context, url, platform, taskDir string) (string, string, string, int64) {
	format := "best"
	if platform == "youtube" {
		format = youtube.FormatSelector("", testYouTubeQuality, "")
	}
	opts := h.ytdlpOpts(platform, format, testDefaultTimeout)
	if err := ytdlp.Download(ctx, url, taskDir, opts); err != nil {
		return db.TestStatusFailed, truncateTestErr(err), "", 0
	}
	return testDirMedia(taskDir)
}

// testX mirrors the production x dispatch: ytdlp first, photo fallback on
// any failure (runXPhotos semantics).
func (h *Handler) testX(ctx context.Context, url, taskDir string) (string, string, string, int64) {
	opts := h.ytdlpOpts("x", "best", testDefaultTimeout)
	err := ytdlp.Download(ctx, url, taskDir, opts)
	if err == nil {
		if status, errMsg, mediaType, size := testDirMedia(taskDir); status == db.TestStatusPassed {
			return status, errMsg, mediaType, size
		}
	}
	_, paths, err := xphotos.DownloadPhotos(ctx, url, taskDir, "", 4)
	if err != nil {
		return db.TestStatusFailed, truncateTestErr(err), "", 0
	}
	if len(paths) == 0 {
		return db.TestStatusFailed, "no photos downloaded", "", 0
	}
	size := int64(0)
	for _, p := range paths {
		size += fileSize(p)
	}
	return db.TestStatusPassed, "", "photos", size
}

// testPinterest mirrors runPinterest: one item is enough to prove the
// scenario (pin extraction + media download) works.
func (h *Handler) testPinterest(ctx context.Context, url, taskDir string) (string, string, string, int64) {
	client := pinterest.NewClient(h.cfg.PinterestCookiesPath, h.runtime.CurrentInt(ctx, "pinterest.timeout_sec", h.cfg.PinterestTimeoutSeconds))
	result, err := client.Download(ctx, url, taskDir, 1,
		h.runtime.CurrentBool(ctx, "pinterest.download_images", h.cfg.PinterestDownloadImages),
		h.runtime.CurrentBool(ctx, "pinterest.download_videos", h.cfg.PinterestDownloadVideos))
	if err != nil {
		return db.TestStatusFailed, truncateTestErr(err), "", 0
	}
	if len(result.Items) == 0 {
		return db.TestStatusFailed, "no media items returned", "", 0
	}
	item := result.Items[0]
	mediaType := "photos"
	if item.MediaType == "video" {
		mediaType = "video"
	}
	return db.TestStatusPassed, "", mediaType, item.FileSize
}

// testReddit mirrors the production reddit scenario minus the Telegram send:
// the thread JSON must parse and, when the post carries media, at least one
// file must land in the temp dir (images natively, video via yt-dlp).
func (h *Handler) testReddit(ctx context.Context, url, taskDir string) (string, string, string, int64) {
	threadID := reddit.ExtractThreadID(url)
	if threadID == "" {
		return db.TestStatusFailed, "no thread id in url", "", 0
	}
	client := reddit.NewClient(h.runtime.CurrentInt(ctx, "reddit.timeout_sec", h.cfg.DownloadTimeoutSeconds),
		cookies.SyncFromMount(h.cfg.RedditCookiesPath, cookies.RedditWritablePath))
	thread, err := client.Thread(ctx, threadID, 3)
	if err != nil {
		return db.TestStatusFailed, truncateTestErr(err), "", 0
	}
	if !thread.HasText() && len(thread.Comments) == 0 && len(thread.Media) == 0 {
		return db.TestStatusFailed, "thread has no content", "", 0
	}
	if len(thread.Media) == 0 {
		return db.TestStatusPassed, "", "text", 0
	}

	m := thread.Media[0]
	if m.Type == "video" {
		vidDir := filepath.Join(taskDir, "video")
		_ = os.MkdirAll(vidDir, 0o755)
		// Reddit hosts video and audio as separate DASH streams; the merge
		// selector is required or the download has no audio track.
		if err := ytdlp.Download(ctx, thread.Permalink, vidDir, h.ytdlpOpts("reddit", "bv*+ba/b", time.Duration(h.cfg.DownloadTimeoutSeconds)*time.Second)); err != nil {
			return db.TestStatusFailed, truncateTestErr(err), "", 0
		}
		return testDirMedia(vidDir)
	}

	imgDir := filepath.Join(taskDir, "img")
	_ = os.MkdirAll(imgDir, 0o755)
	path, err := client.DownloadImage(ctx, m.URL, imgDir)
	if err != nil {
		return db.TestStatusFailed, truncateTestErr(err), "", 0
	}
	info, err := os.Stat(path)
	if err != nil {
		return db.TestStatusFailed, truncateTestErr(err), "", 0
	}
	mediaType := "photos"
	if m.Type == "gif" {
		mediaType = "video"
	}
	return db.TestStatusPassed, "", mediaType, info.Size()
}

// testDirMedia classifies what ytdlp left in the temp dir: a video wins,
// otherwise the images count as a photo set.
func testDirMedia(taskDir string) (string, string, string, int64) {
	files, err := ytdlp.FindMediaFiles(taskDir)
	if err != nil {
		return db.TestStatusFailed, truncateTestErr(err), "", 0
	}
	if video := ytdlp.LargestVideo(files); video != "" {
		return db.TestStatusPassed, "", "video", fileSize(video)
	}
	images := ytdlp.ImageFiles(files)
	if len(images) == 0 {
		return db.TestStatusFailed, "downloaded but no media files found", "", 0
	}
	size := int64(0)
	for _, img := range images {
		size += fileSize(img)
	}
	return db.TestStatusPassed, "", "photos", size
}

func truncateTestErr(err error) string {
	msg := strings.TrimSpace(err.Error())
	if len(msg) > testMaxErrorMessage {
		msg = msg[:testMaxErrorMessage]
	}
	return msg
}
