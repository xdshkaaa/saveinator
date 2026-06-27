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
	imageBody := []byte("fake-jpeg-bytes")
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
	bodyA := []byte("carousel-image-a")
	bodyB := []byte("carousel-image-b")

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
