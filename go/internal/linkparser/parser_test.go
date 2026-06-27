package linkparser

import "testing"

func TestExtractURLsYouTube(t *testing.T) {
	links := ExtractURLs("check https://youtu.be/dQw4w9WgXcQ now")
	if len(links) != 1 || links[0].Platform != PlatformYouTube {
		t.Fatalf("unexpected: %+v", links)
	}
}

func TestExtractURLsXStatusID(t *testing.T) {
	links := ExtractURLs("https://x.com/user/status/1234567890")
	if len(links) != 1 || links[0].XStatusID != "1234567890" {
		t.Fatalf("unexpected: %+v", links)
	}
}

func TestExtractURLsSpotify(t *testing.T) {
	links := ExtractURLs("spotify:track:6rqhFgbbKwnb9MLmUQDhG6")
	if len(links) != 1 || links[0].SpotifyID == "" {
		t.Fatalf("unexpected: %+v", links)
	}
}

func TestIsYouTubeShorts(t *testing.T) {
	if !IsYouTubeShorts("https://www.youtube.com/shorts/abc12345678") {
		t.Fatal("expected shorts")
	}
}
