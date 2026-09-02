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

func TestExtractURLsYandexMusic(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		albumID string
		trackID string
		wantURL string
	}{
		{"album+track with query", "https://music.yandex.ru/album/43378588/track/154402671?ref_id=AD4D3C1C-A5B3-4AF3-B111-771C728895AC&utm_medium=share_link_tg&utm_source=mobile_ios",
			"43378588", "154402671", "https://music.yandex.ru/album/43378588/track/154402671?ref_id=AD4D3C1C-A5B3-4AF3-B111-771C728895AC&utm_medium=share_link_tg&utm_source=mobile_ios"},
		{"bare album+track", "https://music.yandex.ru/album/123456/track/789012",
			"123456", "789012", "https://music.yandex.ru/album/123456/track/789012"},
		{"plain track", "https://music.yandex.ru/track/154402671",
			"", "154402671", "https://music.yandex.ru/track/154402671"},
		{"whole album", "https://music.yandex.ru/album/43378588",
			"43378588", "", "https://music.yandex.ru/album/43378588"},
		{"kz tld", "https://music.yandex.kz/album/1/track/2",
			"1", "2", "https://music.yandex.kz/album/1/track/2"},
		{"com tld www", "https://www.music.yandex.com/track/42",
			"", "42", "https://www.music.yandex.com/track/42"},
		{"trailing slash and punctuation", "смотри https://music.yandex.ru/album/9/track/8/, круто",
			"9", "8", "https://music.yandex.ru/album/9/track/8/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			links := ExtractURLs(tc.url)
			if len(links) != 1 {
				t.Fatalf("expected 1 link, got %d (%+v)", len(links), links)
			}
			l := links[0]
			if l.Platform != PlatformYandexMusic {
				t.Fatalf("platform = %q, want %q", l.Platform, PlatformYandexMusic)
			}
			if l.YandexAlbumID != tc.albumID || l.YandexTrackID != tc.trackID {
				t.Fatalf("ids = (%q, %q), want (%q, %q)", l.YandexAlbumID, l.YandexTrackID, tc.albumID, tc.trackID)
			}
			if l.URL != tc.wantURL {
				t.Fatalf("url = %q, want %q", l.URL, tc.wantURL)
			}
		})
	}
}

func TestExtractURLsYandexMusicDedup(t *testing.T) {
	text := "a https://music.yandex.ru/album/5/track/7 b https://music.yandex.kz/album/5/track/7"
	links := ExtractURLs(text)
	if len(links) != 1 {
		t.Fatalf("expected dedup by track id, got %d (%+v)", len(links), links)
	}
}

func TestExtractURLsInstagram(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		url  string
		want string // expected extracted URL
	}{
		{"post with utm query", "https://www.instagram.com/p/DaBjYIIMEKF/?utm_source=ig_web_copy_link", "https://www.instagram.com/p/DaBjYIIMEKF/?utm_source=ig_web_copy_link"},
		{"reel", "https://www.instagram.com/reel/CxAbC12345/", "https://www.instagram.com/reel/CxAbC12345/"},
		{"mobile", "https://m.instagram.com/p/DaBjYIIMEKF/", "https://m.instagram.com/p/DaBjYIIMEKF/"},
		{"tv", "https://www.instagram.com/tv/CxAbC12345/", "https://www.instagram.com/tv/CxAbC12345/"},
		{"stories", "https://www.instagram.com/stories/username/1234567890/", "https://www.instagram.com/stories/username/1234567890/"},
		{"share", "https://www.instagram.com/share/AbC12345/", "https://www.instagram.com/share/AbC12345/"},
		{"short", "https://instagr.am/p/DaBjYIIMEKF/", "https://instagr.am/p/DaBjYIIMEKF/"},
		{"in text with trailing punctuation", "look at https://www.instagram.com/reel/CxAbC12345/, cool", "https://www.instagram.com/reel/CxAbC12345/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			links := ExtractURLs(tc.url)
			if len(links) != 1 {
				t.Fatalf("expected 1 link, got %d (%+v)", len(links), links)
			}
			if links[0].Platform != PlatformInstagram {
				t.Fatalf("platform = %q, want %q", links[0].Platform, PlatformInstagram)
			}
			if links[0].URL != tc.want {
				t.Fatalf("url = %q, want %q", links[0].URL, tc.want)
			}
		})
	}
}

func TestExtractURLsTwitchClip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		url  string
		want string // expected extracted URL
	}{
		{"channel clip", "https://www.twitch.tv/lenagol0vach/clip/NiceAbstemiousBasenjiLitty-i4Kr9OTU1k0pRZYL", "https://www.twitch.tv/lenagol0vach/clip/NiceAbstemiousBasenjiLitty-i4Kr9OTU1k0pRZYL"},
		{"bare clip path", "https://www.twitch.tv/clip/NiceAbstemiousBasenjiLitty-i4Kr9OTU1k0pRZYL", "https://www.twitch.tv/clip/NiceAbstemiousBasenjiLitty-i4Kr9OTU1k0pRZYL"},
		{"mobile", "https://m.twitch.tv/lenagol0vach/clip/NiceAbstemiousBasenjiLitty-i4Kr9OTU1k0pRZYL", "https://m.twitch.tv/lenagol0vach/clip/NiceAbstemiousBasenjiLitty-i4Kr9OTU1k0pRZYL"},
		{"legacy clips subdomain", "https://clips.twitch.tv/NiceAbstemiousBasenjiLitty-i4Kr9OTU1k0pRZYL", "https://clips.twitch.tv/NiceAbstemiousBasenjiLitty-i4Kr9OTU1k0pRZYL"},
		{"with query", "https://www.twitch.tv/lenagol0vach/clip/NiceAbstemiousBasenjiLitty-i4Kr9OTU1k0pRZYL?filter=clips", "https://www.twitch.tv/lenagol0vach/clip/NiceAbstemiousBasenjiLitty-i4Kr9OTU1k0pRZYL?filter=clips"},
		{"in text with trailing punctuation", "Посмотрите этот клип! lenagol0vach ведет трансляцию Dota 2! https://www.twitch.tv/lenagol0vach/clip/NiceAbstemiousBasenjiLitty-i4Kr9OTU1k0pRZYL", "https://www.twitch.tv/lenagol0vach/clip/NiceAbstemiousBasenjiLitty-i4Kr9OTU1k0pRZYL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			links := ExtractURLs(tc.url)
			if len(links) != 1 {
				t.Fatalf("expected 1 link, got %d (%+v)", len(links), links)
			}
			if links[0].Platform != PlatformTwitch {
				t.Fatalf("platform = %q, want %q", links[0].Platform, PlatformTwitch)
			}
			if links[0].URL != tc.want {
				t.Fatalf("url = %q, want %q", links[0].URL, tc.want)
			}
		})
	}
}

func TestExtractURLsTwitchVODNotMatched(t *testing.T) {
	t.Parallel()
	links := ExtractURLs("https://www.twitch.tv/videos/123456789")
	if len(links) != 1 || links[0].Platform != PlatformUnknown {
		t.Fatalf("VODs are out of scope, got %+v", links)
	}
}

func TestExtractURLsMultilineBatch(t *testing.T) {
	text := `https://vt.tiktok.com/ZSxv29fme/
https://www.youtube.com/shorts/0MEIBEbWSVM?feature=share
https://x.com/solidphono/status/2069500259655413885
https://vt.tiktok.com/ZSC6GCm3S/
https://ru.pinterest.com/pin/811985007859293841/
https://open.spotify.com/track/29YSKt01a9wGNJkPLQG0Kw?si=31980b03a42e4c3a
https://on.soundcloud.com/pffse5BOEisl5gAXNn`

	links := ExtractURLs(text)
	if len(links) != 7 {
		t.Fatalf("expected 7 links, got %d: %+v", len(links), links)
	}

	want := []Platform{
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
	if links[2].XStatusID != "2069500259655413885" {
		t.Fatalf("unexpected x status id: %q", links[2].XStatusID)
	}
}
