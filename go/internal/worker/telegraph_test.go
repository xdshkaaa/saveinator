package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hibiken/asynq"

	"saveinator/internal/queue"
	"saveinator/internal/reddit"
	"saveinator/internal/translate"
)

func newTranslateTask(body []byte) *asynq.Task {
	return asynq.NewTask(queue.TypeTelegraphTranslate, body)
}

func TestChunkRunes(t *testing.T) {
	if got := chunkRunes("short text", maxChunk); len(got) != 1 || got[0] != "short text" {
		t.Fatalf("short text should not be chunked, got %v", got)
	}

	long := strings.Repeat("word ", 2000) // 10k runes
	parts := chunkRunes(long, maxChunk)
	if len(parts) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(parts))
	}
	total := 0
	for _, p := range parts {
		if len([]rune(p)) > maxChunk {
			t.Fatalf("chunk too long: %d runes", len([]rune(p)))
		}
		total += len([]rune(p))
	}
	if total != len([]rune(long)) {
		t.Fatalf("chunks lost content: %d != %d", total, len([]rune(long)))
	}
}

func TestTranslateBlockCyrillicSkipped(t *testing.T) {
	// translate.Google short-circuits all-Cyrillic input without any HTTP
	// call, so this test runs fully offline.
	g := translate.NewGoogle()
	in := "Первый абзац.\n\nВторой абзац."
	if got := translateBlock(context.Background(), g, in); got != in {
		t.Fatalf("translateBlock = %q, want unchanged", got)
	}
}

func TestTranslateBlockEmpty(t *testing.T) {
	g := translate.NewGoogle()
	if got := translateBlock(context.Background(), g, ""); got != "" {
		t.Fatalf("empty input = %q", got)
	}
}

func TestTranslateResultHTML(t *testing.T) {
	ref := &articleRef{URL: "https://telegra.ph/Orig-09-04", Title: "My <post>"}
	got := translateResultHTML(ref, "Мой пост", "https://telegra.ph/RU-09-04")
	if !strings.Contains(got, `<a href="https://telegra.ph/Orig-09-04">My &lt;post&gt;</a>`) {
		t.Fatalf("original link not escaped/present: %q", got)
	}
	if !strings.Contains(got, `🇷🇺 <a href="https://telegra.ph/RU-09-04">Мой пост</a>`) {
		t.Fatalf("ru link missing: %q", got)
	}

	got = translateResultHTML(nil, "Мой пост", "https://telegra.ph/RU-09-04")
	if strings.Contains(got, "Orig") || !strings.Contains(got, "🇷🇺") {
		t.Fatalf("nil ref handling broken: %q", got)
	}
}

func TestClipRuneTitle(t *testing.T) {
	if got := clipRuneTitle("short", 250); got != "short" {
		t.Fatalf("clipRuneTitle = %q", got)
	}
	long := strings.Repeat("а", 300)
	if got := clipRuneTitle(long, 250); len([]rune(got)) != 250 {
		t.Fatalf("clipRuneTitle len = %d", len([]rune(got)))
	}
}

func TestRedditThreadCacheRoundtrip(t *testing.T) {
	h, _, _ := testWorkerHandler(t)
	thread := &reddit.Thread{
		ID: "1abcde", Title: "Cached", Subreddit: "golang",
		Selftext: "hello",
		Comments: []reddit.Comment{{Author: "bob", Score: 3, Body: "hey"}},
	}

	h.cacheRedditThread(context.Background(), thread.ID, thread)
	got := h.cachedRedditThread(context.Background(), thread.ID)
	if got == nil || got.ID != "1abcde" || got.Title != "Cached" || len(got.Comments) != 1 {
		t.Fatalf("cached thread = %+v", got)
	}
	if got.Comments[0].Author != "bob" || got.Comments[0].Body != "hey" {
		t.Fatalf("comments lost: %+v", got.Comments)
	}
}

func TestLoadArticleRefRoundtrip(t *testing.T) {
	h, _, _ := testWorkerHandler(t)
	ctx := context.Background()

	if ref := h.loadArticleRef(ctx, "1abcde", 42); ref != nil {
		t.Fatalf("expected nil ref before store, got %+v", ref)
	}

	data, _ := json.Marshal(articleRef{URL: "https://telegra.ph/X", Title: "T"})
	if err := h.redis.Raw().Set(ctx, "telegraph:page:1abcde:42", data, 0).Err(); err != nil {
		t.Fatal(err)
	}
	ref := h.loadArticleRef(ctx, "1abcde", 42)
	if ref == nil || ref.URL != "https://telegra.ph/X" || ref.Title != "T" {
		t.Fatalf("ref = %+v", ref)
	}
}

func TestTelegraphTranslatePayloadDecode(t *testing.T) {
	p := queue.TelegraphTranslatePayload{
		ThreadID: "1abcde", UserID: 42, ChatID: 7, MessageID: 9, Lang: "ru",
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var back queue.TelegraphTranslatePayload
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back != p {
		t.Fatalf("roundtrip = %+v", back)
	}
}

func TestHandleTelegraphTranslate_invalidPayload(t *testing.T) {
	h, _, _ := testWorkerHandler(t)
	task := newTranslateTask([]byte("not-json"))
	if err := h.handleTelegraphTranslate(context.Background(), task); err == nil {
		t.Fatal("expected unmarshal error")
	}
}
