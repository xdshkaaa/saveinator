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

func TestExtractURLsInstagramPost(t *testing.T) {
	t.Parallel()
	url := "https://www.instagram.com/p/DaBjYIIMEKF/?utm_source=ig_web_copy_link&igsh=NTc4MTIwNjQ2YQ=="
	links := ExtractURLs(url)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].Platform != PlatformInstagram {
		t.Fatalf("platform = %q, want instagram", links[0].Platform)
	}
	if links[0].URL != url {
		t.Fatalf("url = %q", links[0].URL)
	}
}

func TestExtractURLsMultilineBatch(t *testing.T) {
	text := `https://www.instagram.com/reel/DY_nCCclIFx/?igsh=bmdvNzAxbHlnNXd2
https://vt.tiktok.com/ZSxv29fme/
https://www.youtube.com/shorts/0MEIBEbWSVM?feature=share
https://x.com/solidphono/status/2069500259655413885
https://vt.tiktok.com/ZSC6GCm3S/
https://ru.pinterest.com/pin/811985007859293841/
https://open.spotify.com/track/29YSKt01a9wGNJkPLQG0Kw?si=31980b03a42e4c3a
https://on.soundcloud.com/pffse5BOEisl5gAXNn`

	links := ExtractURLs(text)
	if len(links) != 8 {
		t.Fatalf("expected 8 links, got %d: %+v", len(links), links)
	}

	want := []Platform{
		PlatformInstagram,
		PlatformTikTok,
		PlatformYouTube,
		PlatformX,
		PlatformTikTok,
		PlatformPinterest,
		PlatformSpotify,
		PlatformSoundCloud,
	}
	for i, platform := range want {
		if links[i].Platform != platform {
			t.Fatalf("link %d: got %q, want %q (url=%s)", i, links[i].Platform, platform, links[i].URL)
		}
	}
	if links[3].XStatusID != "2069500259655413885" {
		t.Fatalf("unexpected x status id: %q", links[3].XStatusID)
	}
}
