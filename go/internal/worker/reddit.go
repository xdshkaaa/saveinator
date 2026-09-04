package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	"saveinator/internal/cookies"
	"saveinator/internal/locale"
	"saveinator/internal/queue"
	"saveinator/internal/reddit"
	"saveinator/internal/telegraph"
	"saveinator/internal/ytdlp"
)

func (h *Handler) handleReddit(ctx context.Context, t *asynq.Task) error {
	var p queue.DownloadPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	p.Platform = "reddit"
	defer h.releaseLock(ctx, p)
	if h.checkCancelled(ctx, p) {
		return nil
	}
	return h.runReddit(ctx, p)
}

func (h *Handler) runReddit(ctx context.Context, p queue.DownloadPayload) error {
	start := time.Now()
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}

	_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("download.downloading", lang, nil))

	threadID := reddit.ExtractThreadID(p.URL)
	if threadID == "" {
		err := fmt.Errorf("reddit: no thread id in %q", p.URL)
		return h.failReddit(ctx, p, lang, err)
	}

	taskDir, err := os.MkdirTemp("", "saveinator-reddit-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(taskDir)

	client := reddit.NewClient(h.runtime.CurrentInt(ctx, "reddit.timeout_sec", h.cfg.DownloadTimeoutSeconds),
		cookies.SyncFromMount(h.cfg.RedditCookiesPath, cookies.RedditWritablePath))

	fetchCtx, cancel := context.WithTimeout(ctx, time.Duration(h.runtime.CurrentInt(ctx, "reddit.timeout_sec", h.cfg.DownloadTimeoutSeconds))*time.Second)
	thread, err := client.Thread(fetchCtx, threadID, h.redditMaxComments(ctx))
	cancel()
	if err != nil {
		switch {
		case errors.Is(err, reddit.ErrNotFound):
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.not_found", lang, nil))
		case errors.Is(err, reddit.ErrRateLimited):
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.rate_limited", lang, nil))
		default:
			slog.Warn("reddit thread fetch failed", "err", err)
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		}
		recordTaskFailure(queue.TypeReddit)
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "reddit", "failed", 0, err.Error())
		return nil
	}

	// Persist the thread so the translate button can reuse the exact same
	// content (and avoid hammering reddit) later on.
	h.cacheRedditThread(ctx, threadID, thread)

	mediaSent, mediaSize := h.sendRedditMedia(ctx, p, client, thread, taskDir, lang)

	articleErr := h.deliverRedditArticle(ctx, p, thread, lang, mediaSent)
	if !mediaSent && articleErr != nil {
		if articleErr != errRedditArticleDisabled {
			slog.Warn("reddit article failed", "err", articleErr)
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, articleErr))
			_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "reddit", "failed", 0, articleErr.Error())
		} else {
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("reddit.no_content", lang, nil))
			_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "reddit", "failed", 0, "nothing to deliver")
		}
		recordTaskFailure(queue.TypeReddit)
		return nil
	}

	// Media went out. The placeholder goes away on success; when only the
	// article failed it becomes the error notice instead of dying silently.
	if mediaSent && articleErr != nil && articleErr != errRedditArticleDisabled {
		slog.Warn("reddit article failed", "err", articleErr)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, articleErr))
	} else if mediaSent {
		_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
	}

	status, statusErr := "completed", ""
	if articleErr != nil && articleErr != errRedditArticleDisabled {
		statusErr = articleErr.Error()
	}
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "reddit", status, mediaSize, statusErr)
	recordTaskSuccess(queue.TypeReddit, "reddit", start, int64(mediaSize*1024*1024))
	return nil
}

// errRedditArticleDisabled marks "nothing to deliver" cases where neither
// media exists nor the article flow applies.
var errRedditArticleDisabled = errors.New("reddit: article disabled or empty")

// deliverRedditArticle publishes the thread as a Telegraph page and delivers
// the link (as a new message after media, or as an in-place edit). It returns
// errRedditArticleDisabled when the article flow is off or the thread has no
// text/comments at all.
func (h *Handler) deliverRedditArticle(ctx context.Context, p queue.DownloadPayload, thread *reddit.Thread, lang string, mediaSent bool) error {
	if !h.runtime.CurrentBool(ctx, "reddit.telegraph_enabled", true) {
		return errRedditArticleDisabled
	}
	if !thread.HasText() && len(thread.Comments) == 0 {
		return errRedditArticleDisabled
	}

	pageURL, title, err := h.publishRedditArticle(ctx, p, thread, lang)
	if err != nil {
		return err
	}

	text := articleMessageHTML(title, pageURL)
	kb := telegraph.TranslateKeyboard(lang, telegraph.TranslateData{UserID: p.UserID, ThreadID: thread.ID})
	if !h.runtime.CurrentBool(ctx, "reddit.telegraph_translate", true) {
		kb = nil
	}

	if mediaSent {
		_, err = h.sender.SendMessageMarkup(p.ChatID, text, kb)
	} else {
		err = h.sender.EditMessageHTML(p.ChatID, p.MessageID, text, kb)
	}
	return err
}

// publishRedditArticle builds and uploads the Telegraph page and remembers
// the published page (for the translate callback) in Redis.
func (h *Handler) publishRedditArticle(ctx context.Context, p queue.DownloadPayload, thread *reddit.Thread, lang string) (pageURL, title string, err error) {
	token, err := telegraph.ResolveToken(ctx, h.cfg.TelegraphAccessToken, h.cfg.TelegraphAuthorName, h.redis)
	if err != nil {
		return "", "", err
	}

	title, nodes := telegraph.Article(thread, telegraph.ArticleOptions{
		CommentsHeading: locale.Get("telegraph.comments_heading", lang, nil),
		SourceLabel:     locale.Get("telegraph.source_label", lang, nil),
	})

	pageURL, err = telegraph.NewClient().CreatePage(ctx, token, title, nodes, h.cfg.TelegraphAuthorName, "")
	if err != nil {
		return "", "", err
	}

	if ref, err := json.Marshal(articleRef{URL: pageURL, Title: title}); err == nil {
		key := fmt.Sprintf("telegraph:page:%s:%d", thread.ID, p.UserID)
		if err := h.redis.Raw().Set(ctx, key, ref, 7*24*time.Hour).Err(); err != nil {
			slog.Warn("telegraph article ref store failed", "err", err)
		}
	}
	return pageURL, title, nil
}

type articleRef struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// articleMessageHTML renders the "📰 <linked title>" article message body.
// The title is escaped: it comes from untrusted post data.
func articleMessageHTML(title, pageURL string) string {
	return "📰 " + fmt.Sprintf("<a href=%q>%s</a>", pageURL, html.EscapeString(title))
}

// sendRedditMedia downloads and delivers the post's assets: images and gifs
// go out natively, hosted videos via yt-dlp (which merges reddit's separate
// audio stream). Returns whether anything was delivered and its total size.
func (h *Handler) sendRedditMedia(ctx context.Context, p queue.DownloadPayload, client *reddit.Client, thread *reddit.Thread, taskDir, lang string) (bool, float64) {
	if len(thread.Media) == 0 {
		return false, 0
	}
	maxItems := h.redditMaxMediaItems(ctx)
	videoLimit := float64(h.runtime.PlatformMaxFileMB(ctx, "reddit"))
	imageLimit := float64(h.runtime.CurrentInt(ctx, "global.document_limit_mb", h.cfg.SendDocumentLimitMB))

	imgDir := filepath.Join(taskDir, "img")
	vidDir := filepath.Join(taskDir, "video")
	_ = os.MkdirAll(imgDir, 0o755)
	_ = os.MkdirAll(vidDir, 0o755)

	var imagePaths []string
	sentAny := false
	totalBytes := int64(0)
	handled := 0

	for _, m := range thread.Media {
		if handled >= maxItems {
			break
		}
		switch m.Type {
		case "video":
			dlCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
			// Reddit hosts video and audio as separate DASH streams, so the
			// explicit merge format is required — "best" alone often has no
			// audio track at all.
			err := ytdlp.Download(dlCtx, thread.Permalink, vidDir, h.ytdlpOpts("reddit", "bv*+ba/b", 15*time.Minute))
			cancel()
			if err != nil {
				slog.Warn("reddit video download failed", "err", err)
				continue
			}
			path, size, err := newestFileIn(vidDir)
			if err != nil {
				slog.Warn("reddit video file missing", "err", err)
				continue
			}
			if float64(size)/(1024*1024) > videoLimit {
				slog.Warn("reddit video too large", "size", size)
				continue
			}
			if err := h.sendFile(p, path, thread.Title, lang, "reddit", false); err != nil {
				slog.Warn("reddit video send failed", "err", err)
				continue
			}
			totalBytes += size
			sentAny = true
			handled++
		case "image", "gif":
			path, err := client.DownloadImage(ctx, m.URL, imgDir)
			if err != nil {
				slog.Warn("reddit image download failed", "url", m.URL, "err", err)
				continue
			}
			if info, err := os.Stat(path); err == nil {
				if m.Type == "image" && float64(info.Size())/(1024*1024) > imageLimit {
					continue
				}
				totalBytes += info.Size()
			}
			if m.Type == "gif" {
				if err := h.sendFile(p, path, thread.Title, lang, "reddit", true); err != nil {
					slog.Warn("reddit gif send failed", "err", err)
					continue
				}
				sentAny = true
				handled++
				continue
			}
			imagePaths = append(imagePaths, path)
			handled++
		}
	}

	if len(imagePaths) == 1 {
		if err := h.sendFile(p, imagePaths[0], thread.Title, lang, "reddit", false); err != nil {
			slog.Warn("reddit image send failed", "err", err)
		} else {
			sentAny = true
		}
	} else if len(imagePaths) > 1 {
		caption := buildMediaCaption(clipCaption(thread.Title), lang, p.NoWatermark)
		if err := h.sender.SendPhotoAlbum(p.ChatID, imagePaths, caption); err != nil {
			slog.Warn("reddit album send failed", "err", err)
		} else {
			sentAny = true
		}
	}

	return sentAny, float64(totalBytes) / (1024 * 1024)
}

// clipCaption keeps album captions under Telegram's 1024-char caption limit.
func clipCaption(title string) string {
	r := []rune(strings.TrimSpace(title))
	if len(r) > 900 {
		return string(r[:900]) + "…"
	}
	return string(r)
}

// newestFileIn finds the most recently modified regular file in dir,
// ignoring yt-dlp's cookie scratch copy: prepareCookieOptions writes it
// into the same output dir and yt-dlp rewrites it after every run, so it
// would otherwise win the mtime race against the actual download.
func newestFileIn(dir string) (string, int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", 0, err
	}
	var best os.DirEntry
	var bestMod time.Time
	for _, e := range entries {
		if e.IsDir() || e.Name() == "yt-dlp-cookies.txt" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if best == nil || info.ModTime().After(bestMod) {
			best, bestMod = e, info.ModTime()
		}
	}
	if best == nil {
		return "", 0, fmt.Errorf("no files in %s", dir)
	}
	info, err := best.Info()
	if err != nil {
		return "", 0, err
	}
	return filepath.Join(dir, best.Name()), info.Size(), nil
}

func (h *Handler) redditMaxComments(ctx context.Context) int {
	return h.runtime.CurrentInt(ctx, "reddit.max_comments", 10)
}

func (h *Handler) redditMaxMediaItems(ctx context.Context) int {
	return h.runtime.CurrentInt(ctx, "reddit.max_media_items", 10)
}

func (h *Handler) cacheRedditThread(ctx context.Context, threadID string, thread *reddit.Thread) {
	data, err := json.Marshal(thread)
	if err != nil {
		return
	}
	key := fmt.Sprintf("reddit:thread:%s", threadID)
	if err := h.redis.Raw().Set(ctx, key, data, 24*time.Hour).Err(); err != nil {
		slog.Warn("reddit thread cache failed", "err", err)
	}
}

func (h *Handler) failReddit(ctx context.Context, p queue.DownloadPayload, lang string, err error) error {
	slog.Warn("reddit download failed", "err", err)
	recordTaskFailure(queue.TypeReddit)
	_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "reddit", "failed", 0, err.Error())
	return nil
}
