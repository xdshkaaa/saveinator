package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"sync"
	"time"

	"github.com/hibiken/asynq"

	"saveinator/internal/cookies"
	"saveinator/internal/locale"
	"saveinator/internal/queue"
	"saveinator/internal/reddit"
	"saveinator/internal/telegraph"
	"saveinator/internal/translate"
)

func (h *Handler) handleTelegraphTranslate(ctx context.Context, t *asynq.Task) error {
	var p queue.TelegraphTranslatePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}

	thread := h.cachedRedditThread(ctx, p.ThreadID)
	if thread == nil {
		client := reddit.NewClient(h.runtime.CurrentInt(ctx, "reddit.timeout_sec", h.cfg.DownloadTimeoutSeconds),
			cookies.SyncFromMount(h.cfg.RedditCookiesPath, cookies.RedditWritablePath))
		fetchCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		var err error
		thread, err = client.Thread(fetchCtx, p.ThreadID, h.redditMaxComments(ctx))
		cancel()
		if err != nil {
			slog.Warn("translate refetch failed", "thread", p.ThreadID, "err", err)
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
			recordTaskFailure(queue.TypeTelegraphTranslate)
			return nil
		}
	}

	translated := h.translateThread(ctx, thread)

	token, err := telegraph.ResolveToken(ctx, h.cfg.TelegraphAccessToken, h.cfg.TelegraphAuthorName, h.redis)
	if err != nil {
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		recordTaskFailure(queue.TypeTelegraphTranslate)
		return nil
	}

	title, nodes := telegraph.Article(translated, telegraph.ArticleOptions{
		CommentsHeading: locale.Get("telegraph.comments_heading", lang, nil),
		SourceLabel:     locale.Get("telegraph.source_label", lang, nil),
	})
	// The RU page is marked in the title; keep it within Telegraph's 256-char
	// limit after adding the suffix.
	ruTitle := clipRuneTitle(title, 250) + " (RU)"

	ruURL, err := telegraph.NewClient().CreatePage(ctx, token, ruTitle, nodes, h.cfg.TelegraphAuthorName, "")
	if err != nil {
		slog.Warn("telegraph translate page failed", "err", err)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("telegraph.failed", lang, nil))
		recordTaskFailure(queue.TypeTelegraphTranslate)
		return nil
	}

	text := translateResultHTML(h.loadArticleRef(ctx, p.ThreadID, p.UserID), translated.Title, ruURL)
	kb := telegraph.TranslatedKeyboard(lang, ruURL)
	if err := h.sender.EditMessageHTML(p.ChatID, p.MessageID, text, kb); err != nil {
		slog.Warn("telegraph translate edit failed", "err", err)
		recordTaskFailure(queue.TypeTelegraphTranslate)
		return nil
	}
	recordTaskSuccess(queue.TypeTelegraphTranslate, "", time.Now(), 0)
	return nil
}

// translateResultHTML rebuilds the article message: the original link (from
// the Redis ref when available) plus the fresh RU version. Titles are
// escaped: they come from untrusted post data.
func translateResultHTML(ref *articleRef, ruTitle, ruURL string) string {
	text := ""
	if ref != nil && ref.URL != "" {
		text = articleMessageHTML(ref.Title, ref.URL) + "\n\n"
	}
	return text + "🇷🇺 " + fmt.Sprintf("<a href=%q>%s</a>", ruURL, html.EscapeString(ruTitle))
}

// clipRuneTitle cuts a rune-safe title to max runes.
func clipRuneTitle(title string, max int) string {
	r := []rune(title)
	if len(r) > max {
		return string(r[:max])
	}
	return title
}

// cachedRedditThread reads the thread JSON cached by the reddit worker.
func (h *Handler) cachedRedditThread(ctx context.Context, threadID string) *reddit.Thread {
	key := fmt.Sprintf("reddit:thread:%s", threadID)
	data, err := h.redis.Raw().Get(ctx, key).Bytes()
	if err != nil {
		return nil
	}
	var thread reddit.Thread
	if err := json.Unmarshal(data, &thread); err != nil {
		return nil
	}
	return &thread
}

// loadArticleRef reads the published-article reference stored by the reddit
// worker so the translated message can keep the original link.
func (h *Handler) loadArticleRef(ctx context.Context, threadID string, userID int64) *articleRef {
	key := fmt.Sprintf("telegraph:page:%s:%d", threadID, userID)
	data, err := h.redis.Raw().Get(ctx, key).Bytes()
	if err != nil {
		return nil
	}
	var ref articleRef
	if err := json.Unmarshal(data, &ref); err != nil || ref.URL == "" {
		return nil
	}
	return &ref
}

// translateThread translates title, post text and comment bodies to Russian.
// The translator skips already-Cyrillic text and falls back to the original
// on errors, so a partial Google outage degrades gracefully.
func (h *Handler) translateThread(ctx context.Context, t *reddit.Thread) *reddit.Thread {
	out := *t
	out.Title = translateBlock(ctx, h.translator, t.Title)
	out.Selftext = translateBlock(ctx, h.translator, t.Selftext)
	out.Comments = make([]reddit.Comment, len(t.Comments))
	var wg sync.WaitGroup
	for i, c := range t.Comments {
		wg.Add(1)
		go func(i int, c reddit.Comment) {
			defer wg.Done()
			c.Body = translateBlock(ctx, h.translator, c.Body)
			out.Comments[i] = c
		}(i, c)
	}
	wg.Wait()
	return &out
}

// maxChunk runes per Google request: the keyless endpoint rejects URLs past
// roughly 5k characters, stay well below that.
const maxChunk = 3500

// translateBlock translates one markdown-ish text block. Long texts are split
// into chunks; short texts go through one-by-one (the translator's cache
// dedups repeats).
func translateBlock(ctx context.Context, g *translate.Google, text string) string {
	if text == "" {
		return text
	}
	var chunks []string
	for _, p := range reddit.Paragraphs(text) {
		chunks = append(chunks, chunkRunes(p, maxChunk)...)
	}
	translated := make([]string, len(chunks))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, chunk := range chunks {
		wg.Add(1)
		go func(i int, chunk string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			translated[i] = g.Text(ctx, chunk)
		}(i, chunk)
	}
	wg.Wait()
	return joinParagraphs(translated)
}

// chunkRunes splits s into <=max-rune pieces, preferring to break at spaces.
func chunkRunes(s string, max int) []string {
	r := []rune(s)
	if len(r) <= max {
		return []string{s}
	}
	var out []string
	for len(r) > max {
		cut := max
		for i := max; i > max/2; i-- {
			if r[i-1] == ' ' {
				cut = i
				break
			}
		}
		out = append(out, string(r[:cut]))
		r = r[cut:]
	}
	if len(r) > 0 {
		out = append(out, string(r))
	}
	return out
}

func joinParagraphs(parts []string) string {
	out := ""
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i > 0 && out != "" {
			out += "\n\n"
		}
		out += p
	}
	return out
}
