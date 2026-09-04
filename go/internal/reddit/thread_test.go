package reddit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const galleryThreadJSON = `[
  {"data": {"children": [{"kind": "t3", "data": {
      "id": "1abcde",
      "title": "My test post",
      "author": "alice",
      "subreddit": "golang",
      "selftext": "Line one **bold**.\n\nCheck [docs](https://go.dev).",
      "score": 42,
      "num_comments": 5,
      "permalink": "/r/golang/comments/1abcde/my_test_post/",
      "is_gallery": true,
      "gallery_data": {"items": [{"media_id": "img1"}, {"media_id": "img2"}, {"media_id": "img3"}]},
      "media_metadata": {
        "img1": {"status": "valid", "e": "Image", "s": {"u": "https://preview.redd.it/one.jpg?width=10&amp;height=2"}},
        "img2": {"status": "valid", "e": "AnimatedImage", "s": {"mp4": "https://preview.redd.it/two.mp4"}},
        "img3": {"status": "failed", "e": "Image", "s": {"u": "https://preview.redd.it/three.jpg"}}
      }
  }}]}},
  {"data": {"children": [
      {"kind": "t1", "data": {"author": "bob", "score": 10, "body": "Nice post!"}},
      {"kind": "t1", "data": {"author": "[deleted]", "score": 3, "body": "[removed]"}},
      {"kind": "t1", "data": {"author": "carol", "score": 1, "body": ""}},
      {"kind": "more", "data": {"count": 3}}
  ]}}
]`

func TestParseThreadGallery(t *testing.T) {
	thread, err := parseThread([]byte(galleryThreadJSON))
	if err != nil {
		t.Fatalf("parseThread: %v", err)
	}
	if thread.ID != "1abcde" || thread.Title != "My test post" || thread.Author != "alice" || thread.Subreddit != "golang" {
		t.Fatalf("post fields = %+v", thread)
	}
	if thread.Score != 42 || thread.NumComments != 5 {
		t.Fatalf("scores = %d/%d", thread.Score, thread.NumComments)
	}
	if thread.Permalink != "https://www.reddit.com/r/golang/comments/1abcde/my_test_post/" {
		t.Fatalf("permalink = %q", thread.Permalink)
	}
	if !thread.HasText() {
		t.Fatal("expected HasText")
	}
	wantMedia := []Media{
		{Type: "image", URL: "https://preview.redd.it/one.jpg?width=10&height=2"},
		{Type: "gif", URL: "https://preview.redd.it/two.mp4"},
	}
	if len(thread.Media) != len(wantMedia) {
		t.Fatalf("media = %+v, want %+v", thread.Media, wantMedia)
	}
	for i, m := range wantMedia {
		if thread.Media[i] != m {
			t.Fatalf("media[%d] = %+v, want %+v", i, thread.Media[i], m)
		}
	}
	if len(thread.Comments) != 1 || thread.Comments[0].Author != "bob" || thread.Comments[0].Score != 10 || thread.Comments[0].Body != "Nice post!" {
		t.Fatalf("comments = %+v", thread.Comments)
	}
}

func TestParseThreadVideoPost(t *testing.T) {
	body := []byte(`[
      {"data": {"children": [{"kind": "t3", "data": {
          "id": "vid1", "title": "A video", "author": "u1", "subreddit": "videos",
          "is_video": true, "permalink": "/r/videos/comments/vid1/a_video/"
      }}]}},
      {"data": {"children": []}}
]`)
	thread, err := parseThread(body)
	if err != nil {
		t.Fatalf("parseThread: %v", err)
	}
	if len(thread.Media) != 1 || thread.Media[0].Type != "video" || thread.Media[0].URL != "https://www.reddit.com/r/videos/comments/vid1/a_video/" {
		t.Fatalf("media = %+v", thread.Media)
	}
	if !thread.HasMedia() {
		t.Fatal("expected HasMedia")
	}
}

func TestParseThreadDirectImage(t *testing.T) {
	body := []byte(`[
      {"data": {"children": [{"kind": "t3", "data": {
          "id": "img1", "title": "A picture", "author": "u1", "subreddit": "pics",
          "url_overridden_by_dest": "https://i.redd.it/photo.PNG",
          "post_hint": "image", "permalink": "/r/pics/comments/img1/a_picture/"
      }}]}},
      {"data": {"children": []}}
]`)
	thread, err := parseThread(body)
	if err != nil {
		t.Fatalf("parseThread: %v", err)
	}
	if len(thread.Media) != 1 || thread.Media[0].Type != "image" || thread.Media[0].URL != "https://i.redd.it/photo.PNG" {
		t.Fatalf("media = %+v", thread.Media)
	}
}

func TestParseThreadNotFound(t *testing.T) {
	if _, err := parseThread([]byte(`[]`)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if _, err := parseThread([]byte(`not json`)); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestExtractThreadID(t *testing.T) {
	cases := map[string]string{
		"https://www.reddit.com/r/golang/comments/1abcde/some_slug/?utm=x": "1abcde",
		"https://old.reddit.com/r/golang/comments/1abcde/some_slug/":       "1abcde",
		"https://www.reddit.com/comments/1zz9zz/":                          "1zz9zz",
		"https://redd.it/1abcde":                                           "1abcde",
		"https://www.redd.it/1abcde/":                                      "1abcde",
		"https://example.com/comments/1abcde/":                             "",
	}
	for url, want := range cases {
		if got := ExtractThreadID(url); got != want {
			t.Errorf("ExtractThreadID(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestToPlainText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"[docs](https://go.dev)", "docs (https://go.dev)"},
		{"**bold** and *ital*", "bold and ital"},
		{"__under__ ~~strike~~ `code`", "under strike code"},
		{"# Header\n> quoted\nplain", "Header\nquoted\nplain"},
		{"name_with_underscores stays", "name_with_underscores stays"},
	}
	for _, tc := range cases {
		if got := ToPlainText(tc.in); got != tc.want {
			t.Errorf("ToPlainText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParagraphs(t *testing.T) {
	got := Paragraphs("first\n\nsecond\n\n\n\nthird line\ncontinued")
	want := []string{"first", "second", "third line continued"}
	if len(got) != len(want) {
		t.Fatalf("paragraphs = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("paragraphs = %+v, want %+v", got, want)
		}
	}
	if len(Paragraphs("  \n\n ")) != 0 {
		t.Fatal("expected no paragraphs for whitespace text")
	}
}

func TestParseThreadAnimatedPreview(t *testing.T) {
	// Real reddit shape: "preview" is an object wrapping the images array.
	body := `[
	  {"data": {"children": [{"kind": "t3", "data": {
	      "id": "2fghij",
	      "title": "gif post",
	      "author": "bob",
	      "subreddit": "golang",
	      "permalink": "/r/golang/comments/2fghij/gif_post/",
	      "preview": {
	        "images": [{
	          "source": {"url": "https://preview.redd.it/x.png", "width": 640},
	          "resolutions": [{"url": "https://preview.redd.it/x.png?width=108"}],
	          "variants": {"mp4": {"source": {"url": "https://preview.redd.it/x.gif?format=mp4"}}}
	        }],
	        "enabled": true
	      }
	  }}]}},
	  {"data": {"children": []}}
	]`

	thread, err := parseThread([]byte(body))
	if err != nil {
		t.Fatalf("parseThread: %v", err)
	}
	want := []Media{{Type: "gif", URL: "https://preview.redd.it/x.gif?format=mp4"}}
	if len(thread.Media) != 1 || thread.Media[0] != want[0] {
		t.Fatalf("media = %+v, want %+v", thread.Media, want)
	}
}

func TestDownloadImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("fakejpegdata"))
	}))
	defer srv.Close()

	c := NewClient(5, "")
	dir := t.TempDir()
	path, err := c.DownloadImage(context.Background(), srv.URL+"/pic.jpg", dir)
	if err != nil {
		t.Fatalf("DownloadImage: %v", err)
	}
	base := filepath.Base(path)
	if len(base) <= 4 || base[:4] != "img_" || filepath.Ext(path) != ".jpg" {
		t.Fatalf("unexpected name/ext: %q", path)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "fakejpegdata" {
		t.Fatalf("file content = %q, err = %v", data, err)
	}
}

func TestDownloadImageHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewClient(5, "")
	if _, err := c.DownloadImage(context.Background(), srv.URL+"/pic.jpg", t.TempDir()); err == nil {
		t.Fatal("expected error on 403")
	}
}
