package telegraph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"saveinator/internal/reddit"
)

func sampleThread() *reddit.Thread {
	return &reddit.Thread{
		ID: "1abcde", Title: "My test post", Author: "alice", Subreddit: "golang",
		Selftext:    "First paragraph.\n\nSecond one with [link](https://go.dev).",
		Score:       42, NumComments: 2,
		Permalink:   "https://www.reddit.com/r/golang/comments/1abcde/my_test_post/",
		Comments: []reddit.Comment{
			{Author: "bob", Score: 10, Body: "Nice post!"},
			{Author: "carol", Score: 2, Body: "Agreed.\n\nAlso: good."},
		},
	}
}

func TestArticle(t *testing.T) {
	title, nodes := Article(sampleThread(), ArticleOptions{
		CommentsHeading: "Top comments",
		SourceLabel:     "Source",
	})
	if title != "My test post" {
		t.Fatalf("title = %q", title)
	}
	if nodes[0].Tag != "p" || nodes[0].Children[0].(Node).Tag != "b" {
		t.Fatalf("meta line = %+v", nodes[0])
	}
	if nodes[1].Tag != "p" {
		t.Fatalf("source line = %+v", nodes[1])
	}

	tags := make([]string, 0, len(nodes))
	for _, n := range nodes {
		tags = append(tags, n.Tag)
	}
	joined := strings.Join(tags, ",")
	// post body, heading and comment blocks must all be present
	for _, want := range []string{"hr", "h3", "blockquote"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in tags %v", want, tags)
		}
	}
}

func TestArticleLongTitleTruncated(t *testing.T) {
	thread := sampleThread()
	thread.Title = strings.Repeat("а", 300)
	title, _ := Article(thread, ArticleOptions{})
	if len([]rune(title)) != maxTitleLen || !strings.HasSuffix(title, "…") {
		t.Fatalf("title len = %d, suffix ok = %v", len([]rune(title)), strings.HasSuffix(title, "…"))
	}
}

func TestArticleTrimsOversizeContent(t *testing.T) {
	thread := sampleThread()
	thread.Comments = nil
	big := strings.Repeat("x", 4096)
	thread.Selftext = strings.Repeat(big+"\n\n", 40) // ~160KB of text

	title, nodes := Article(thread, ArticleOptions{})
	if title == "" {
		t.Fatal("empty title")
	}
	if size := sizeOf(nodes); size > maxContentLen {
		t.Fatalf("content size = %d, want <= %d", size, maxContentLen)
	}
	if len(nodes) == 0 {
		t.Fatal("all content trimmed away")
	}
}

func TestArticleJSONShape(t *testing.T) {
	_, nodes := Article(sampleThread(), ArticleOptions{CommentsHeading: "Comments", SourceLabel: "Source"})
	b, err := json.Marshal(nodes)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"tag":"blockquote"`) || !strings.Contains(string(b), `"href":"https://www.reddit.com`) {
		t.Fatalf("unexpected json: %s", b)
	}
}

func TestTranslateCallbackRoundtrip(t *testing.T) {
	data := TranslateCallbackData(123456789, "1abcde")
	if data != "tg:tr:123456789:1abcde" {
		t.Fatalf("data = %q", data)
	}
	got, ok := ParseTranslate(data)
	if !ok || got.UserID != 123456789 || got.ThreadID != "1abcde" {
		t.Fatalf("parsed = %+v, ok = %v", got, ok)
	}

	for _, bad := range []string{
		"", "tg:tr:1abcde", "tg:xx:1:1abcde", "tg:tr:abc:1abcde",
		"tg:tr:0:1abcde", "tg:tr:123456789:", "xx:tr:123456789:1abcde",
	} {
		if _, ok := ParseTranslate(bad); ok {
			t.Errorf("ParseTranslate(%q) should fail", bad)
		}
	}
}

func TestClientCreateAccountAndPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/createAccount":
			_ = r.ParseForm()
			if r.Form.Get("short_name") == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"access_token":"tok123"}}`))
		case "/createPage":
			_ = r.ParseForm()
			var content []Node
			if err := json.Unmarshal([]byte(r.Form.Get("content")), &content); err != nil {
				t.Errorf("bad content json: %v", err)
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"url":"https://telegra.ph/Slug-09-04","path":"/Slug-09-04"}}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	oldBase := apiBase
	apiBase = srv.URL
	defer func() { apiBase = oldBase }()

	c := NewClient()
	token, err := c.CreateAccount(context.Background(), "saveinator", "tester")
	if err != nil || token != "tok123" {
		t.Fatalf("token = %q, err = %v", token, err)
	}

	title, nodes := Article(sampleThread(), ArticleOptions{CommentsHeading: "Comments"})
	pageURL, err := c.CreatePage(context.Background(), token, title, nodes, "tester", "")
	if err != nil || pageURL != "https://telegra.ph/Slug-09-04" {
		t.Fatalf("pageURL = %q, err = %v", pageURL, err)
	}
}

func TestClientAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"error":"TOKEN_INVALID"}`))
	}))
	defer srv.Close()

	oldBase := apiBase
	apiBase = srv.URL
	defer func() { apiBase = oldBase }()

	c := NewClient()
	if _, err := c.CreatePage(context.Background(), "bad", "t", []Node{HR()}, "", ""); err == nil {
		t.Fatal("expected error")
	}
}
