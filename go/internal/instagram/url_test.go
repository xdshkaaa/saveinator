package instagram

import "testing"

func TestKindFromURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		url  string
		want Kind
	}{
		{"https://www.instagram.com/p/DaBjYIIMEKF/?utm_source=ig_web_copy_link", KindPost},
		{"https://www.instagram.com/reel/DY_nCCclIFx/", KindReel},
		{"https://www.instagram.com/reels/DY_nCCclIFx/", KindReel},
		{"https://www.instagram.com/share/p/BAEo123abc/", KindPost},
		{"https://www.instagram.com/share/reel/BAEo123abc/", KindReel},
		{"https://www.instagram.com/stories/username/1234567890123456789/", KindStory},
		{"https://instagr.am/p/ABCdef12345/", KindPost},
		{"https://www.instagram.com/tv/ABCdef12345/", KindPost},
	}
	for _, tc := range tests {
		if got := KindFromURL(tc.url); got != tc.want {
			t.Fatalf("KindFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestExtractShortcode(t *testing.T) {
	t.Parallel()
	url := "https://www.instagram.com/p/DaBjYIIMEKF/?utm_source=ig_web_copy_link&igsh=NTc4MTIwNjQ2YQ=="
	if got := ExtractShortcode(url); got != "DaBjYIIMEKF" {
		t.Fatalf("ExtractShortcode() = %q, want DaBjYIIMEKF", got)
	}
}

func TestIsPhotoPostURL(t *testing.T) {
	t.Parallel()
	if !IsPhotoPostURL("https://www.instagram.com/p/DaBjYIIMEKF/") {
		t.Fatal("expected photo post URL")
	}
	if IsPhotoPostURL("https://www.instagram.com/reel/DY_nCCclIFx/") {
		t.Fatal("reel should not be a photo post URL")
	}
}
