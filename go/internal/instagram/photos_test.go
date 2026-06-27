package instagram

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPhotoClientDownloadPhotos(t *testing.T) {
	t.Parallel()
	imageBody := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 'f', 'a', 'k', 'e'}
	var requestCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if !strings.Contains(r.URL.Path, "/media/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(imageBody)
	}))
	defer srv.Close()

	client := &PhotoClient{
		http: srv.Client(),
		mediaURL: func(shortcode string, index int) string {
			url := srv.URL + "/p/" + shortcode + "/media/?size=l"
			if index > 1 {
				url += "&index=" + itoa(index)
			}
			return url
		},
	}
	dir := t.TempDir()

	result, paths, err := client.DownloadPhotos(context.Background(), "https://www.instagram.com/p/TESTSHORT/", dir, 5)
	if err != nil {
		t.Fatalf("DownloadPhotos() err = %v", err)
	}
	if result.Shortcode != "TESTSHORT" {
		t.Fatalf("shortcode = %q", result.Shortcode)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 photo (duplicate stop), got %d requests=%d paths=%v", len(paths), requestCount, paths)
	}
	if _, err := os.Stat(paths[0]); err != nil {
		t.Fatalf("photo file missing: %v", err)
	}
}

func TestPhotoClientDownloadPhotos_authRequired(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := &PhotoClient{
		http: srv.Client(),
		mediaURL: func(shortcode string, index int) string {
			return srv.URL + "/p/" + shortcode + "/media/?size=l"
		},
	}

	_, _, err := client.DownloadPhotos(context.Background(), "https://www.instagram.com/p/TESTSHORT/", t.TempDir(), 1)
	if !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("expected auth error, got %v", err)
	}
}

func TestPhotoClientDownloadPhotos_carousel(t *testing.T) {
	t.Parallel()
	bodyA := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 'a'}
	bodyB := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 'b'}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		switch r.URL.Query().Get("index") {
		case "2":
			_, _ = w.Write(bodyB)
		default:
			_, _ = w.Write(bodyA)
		}
	}))
	defer srv.Close()

	client := &PhotoClient{
		http: srv.Client(),
		mediaURL: func(shortcode string, index int) string {
			url := srv.URL + "/p/" + shortcode + "/media/?size=l"
			if index > 1 {
				url += "&index=" + itoa(index)
			}
			return url
		},
	}

	paths, err := func() ([]string, error) {
		_, paths, err := client.DownloadPhotos(context.Background(), "https://www.instagram.com/p/CAROUSEL1/", t.TempDir(), 5)
		return paths, err
	}()
	if err != nil {
		t.Fatalf("DownloadPhotos() err = %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 carousel photos, got %d", len(paths))
	}
	if filepath.Base(paths[0]) != "photo_1.jpg" || filepath.Base(paths[1]) != "photo_2.jpg" {
		t.Fatalf("unexpected paths: %v", paths)
	}
}

func TestPhotoClientDownloadPhotos_requiresReferer(t *testing.T) {
	t.Parallel()
	imageBody := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}
	shortcode := "DZcRZBQI1i6"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != postReferer(shortcode) {
			http.Error(w, "connection reset simulation", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(imageBody)
	}))
	defer srv.Close()

	client := &PhotoClient{
		http: srv.Client(),
		mediaURL: func(shortcode string, index int) string {
			return srv.URL + "/p/" + shortcode + "/media/?size=l"
		},
	}

	result, paths, err := client.DownloadPhotos(context.Background(), "https://www.instagram.com/p/"+shortcode+"/", t.TempDir(), 1)
	if err != nil {
		t.Fatalf("DownloadPhotos() err = %v", err)
	}
	if result.Shortcode != shortcode {
		t.Fatalf("shortcode = %q", result.Shortcode)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 photo, got %d", len(paths))
	}
}

func TestPhotoClientDownloadPhotos_wrongContentType(t *testing.T) {
	t.Parallel()
	imageBody := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(imageBody)
	}))
	defer srv.Close()

	client := &PhotoClient{
		http: srv.Client(),
		mediaURL: func(shortcode string, index int) string {
			return srv.URL + "/p/" + shortcode + "/media/?size=l"
		},
	}

	_, paths, err := client.DownloadPhotos(context.Background(), "https://www.instagram.com/p/DZcRZBQI1i6/", t.TempDir(), 1)
	if err != nil {
		t.Fatalf("DownloadPhotos() err = %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 photo despite text/html content-type, got %d", len(paths))
	}
}

func TestCookieDomainMatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		domain string
		host   string
		want   bool
	}{
		{domain: ".instagram.com", host: "www.instagram.com", want: true},
		{domain: "instagram.com", host: "www.instagram.com", want: true},
		{domain: ".facebook.com", host: "www.instagram.com", want: false},
		{domain: ".google.com", host: "www.google.com", want: true},
	}
	for _, tc := range tests {
		if got := cookieDomainMatches(tc.domain, tc.host); got != tc.want {
			t.Fatalf("cookieDomainMatches(%q, %q) = %v, want %v", tc.domain, tc.host, got, tc.want)
		}
	}
}

func TestLoadCookies_filtersByDomain(t *testing.T) {
	t.Parallel()
	cookieFile := filepath.Join(t.TempDir(), "cookies.txt")
	content := strings.Join([]string{
		"# Netscape HTTP Cookie File",
		".instagram.com\tTRUE\t/\tTRUE\t0\tsessionid\tig_session",
		".google.com\tTRUE\t/\tTRUE\t0\tSID\tgoogle_session",
		"www.instagram.com\tFALSE\t/\tTRUE\t0\tcsrftoken\tcsrf",
	}, "\n")
	if err := os.WriteFile(cookieFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodGet, "https://www.instagram.com/p/TEST/media/?size=l", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &PhotoClient{cookiesPath: cookieFile}
	client.loadCookies(req, "www.instagram.com")

	if len(req.Cookies()) != 2 {
		t.Fatalf("expected 2 instagram cookies, got %d: %v", len(req.Cookies()), req.Cookies())
	}
	names := map[string]struct{}{}
	for _, c := range req.Cookies() {
		names[c.Name] = struct{}{}
	}
	if _, ok := names["sessionid"]; !ok {
		t.Fatal("missing sessionid cookie")
	}
	if _, ok := names["csrftoken"]; !ok {
		t.Fatal("missing csrftoken cookie")
	}
	if _, ok := names["SID"]; ok {
		t.Fatal("google cookie should be filtered out")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
