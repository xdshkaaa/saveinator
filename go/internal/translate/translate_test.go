package translate

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTextTranslatesViaGoogle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("tl"); got != "ru" {
			t.Errorf("tl = %q, want ru", got)
		}
		if got := r.URL.Query().Get("q"); got != "Hello world" {
			t.Errorf("q = %q, want Hello world", got)
		}
		_, _ = w.Write([]byte(`[[["Привет мир","Hello world",null,null,10]],null,"en",null,null,null,0.7]`))
	}))
	defer srv.Close()

	orig := GoogleTranslateAPI
	GoogleTranslateAPI = srv.URL
	defer func() { GoogleTranslateAPI = orig }()

	g := NewGoogle()
	got := g.Text(context.Background(), "Hello world")
	if got != "Привет мир" {
		t.Fatalf("Text() = %q, want Привет мир", got)
	}
}

func TestTextReturnsOriginalOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	orig := GoogleTranslateAPI
	GoogleTranslateAPI = srv.URL
	defer func() { GoogleTranslateAPI = orig }()

	g := NewGoogle()
	if got := g.Text(context.Background(), "Hello"); got != "Hello" {
		t.Fatalf("Text() = %q, want original on HTTP error", got)
	}
}

func TestTextSkipsRussianInput(t *testing.T) {
	g := NewGoogle()
	for _, in := range []string{"", "Привет мир", "Как дела?", "Проверка 123"} {
		if got := g.Text(context.Background(), in); got != strings.TrimSpace(in) {
			t.Fatalf("Text(%q) = %q, want unchanged", in, got)
		}
	}
}

func TestTextCachesResults(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = fmt.Fprintf(w, `[[["Переведено %d","src",null,null,10]],null,"en",null,null,null,0.7]`, calls)
	}))
	defer srv.Close()

	orig := GoogleTranslateAPI
	GoogleTranslateAPI = srv.URL
	defer func() { GoogleTranslateAPI = orig }()

	g := NewGoogle()
	if g.Text(context.Background(), "Once") != "Переведено 1" {
		t.Fatal("first call failed")
	}
	if g.Text(context.Background(), "Once") != "Переведено 1" {
		t.Fatal("cached call must not hit the server")
	}
	if calls != 1 {
		t.Fatalf("server calls = %d, want 1", calls)
	}
}

func TestLooksRussian(t *testing.T) {
	for _, in := range []string{"Привет", "Привет, мир!", "Привет, мир! Это тест на русском языке."} {
		if !looksRussian(in) {
			t.Errorf("looksRussian(%q) = false, want true", in)
		}
	}
	for _, in := range []string{"Hello", "Hello world", "こんにちは", ""} {
		if looksRussian(in) {
			t.Errorf("looksRussian(%q) = true, want false", in)
		}
	}
}
