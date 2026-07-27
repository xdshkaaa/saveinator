package youtube

import "testing"

// sampleMeta mirrors the shape of `yt-dlp -J` for a 1080p landscape upload:
// progressive 360p, video-only ladder up to 1080p, two audio tracks.
const sampleMeta = `{
  "id": "rhkpz9Ud4iU",
  "title": "ЗАЙКА (lyric video)",
  "uploader": "МЭЙБИ БЭЙБИ",
  "duration": 151.0,
  "formats": [
    {"format_id": "140", "ext": "m4a", "vcodec": "none", "acodec": "mp4a.40.2", "abr": 129.0, "filesize": 2400000},
    {"format_id": "251", "ext": "webm", "vcodec": "none", "acodec": "opus", "abr": 140.0, "filesize": 2200000},
    {"format_id": "18",  "ext": "mp4",  "vcodec": "avc1.42001E", "acodec": "mp4a.40.2", "width": 640,  "height": 360,  "filesize": 5000000},
    {"format_id": "134", "ext": "mp4",  "vcodec": "avc1.4d401e", "acodec": "none", "width": 640,  "height": 360,  "filesize": 4000000},
    {"format_id": "135", "ext": "mp4",  "vcodec": "avc1.4d401e", "acodec": "none", "width": 854,  "height": 480,  "filesize": 13600000},
    {"format_id": "136", "ext": "mp4",  "vcodec": "avc1.4d401f", "acodec": "none", "width": 1280, "height": 720,  "filesize": 22600000},
    {"format_id": "247", "ext": "webm", "vcodec": "vp9",         "acodec": "none", "width": 1280, "height": 720,  "filesize": 18000000},
    {"format_id": "137", "ext": "mp4",  "vcodec": "avc1.640028", "acodec": "none", "width": 1920, "height": 1080, "filesize": 99600000}
  ]
}`

func parseSample(t *testing.T, raw string) *Meta {
	t.Helper()
	meta, err := ParseMeta([]byte(raw))
	if err != nil {
		t.Fatalf("ParseMeta: %v", err)
	}
	return meta
}

func TestParseMeta(t *testing.T) {
	meta := parseSample(t, sampleMeta)
	if meta.ID != "rhkpz9Ud4iU" || meta.Title != "ЗАЙКА (lyric video)" {
		t.Fatalf("unexpected identity: %q %q", meta.ID, meta.Title)
	}
	if meta.Author() != "МЭЙБИ БЭЙБИ" {
		t.Fatalf("unexpected author: %q", meta.Author())
	}
	if meta.DurationSec() != 151 {
		t.Fatalf("unexpected duration: %d", meta.DurationSec())
	}
}

func TestOptionsOffersOnlyExistingQualities(t *testing.T) {
	meta := parseSample(t, sampleMeta)
	opts := Options(meta, DefaultHeights())

	var heights []int
	for _, o := range opts {
		heights = append(heights, o.Height)
	}
	want := []int{360, 480, 720, 1080}
	if len(heights) != len(want) {
		t.Fatalf("got heights %v, want %v", heights, want)
	}
	for i := range want {
		if heights[i] != want[i] {
			t.Fatalf("got heights %v, want %v", heights, want)
		}
	}
}

func TestOptionsPicksProgressiveAndMergedFormats(t *testing.T) {
	meta := parseSample(t, sampleMeta)
	opts := Options(meta, DefaultHeights())

	byHeight := make(map[int]Option, len(opts))
	for _, o := range opts {
		byHeight[o.Height] = o
	}

	// 360p has a progressive format, so no merge and no added audio size.
	if got := byHeight[360]; got.FormatID != "18" || got.SizeBytes != 5000000 {
		t.Fatalf("360p: got %+v", got)
	}
	// 720p must prefer h264 over the smaller vp9 so the merge stays mp4, and
	// must pair with the m4a track rather than opus.
	if got := byHeight[720]; got.FormatID != "136+140" || got.SizeBytes != 22600000+2400000 {
		t.Fatalf("720p: got %+v", got)
	}
	if got := byHeight[1080]; got.FormatID != "137+140" {
		t.Fatalf("1080p: got %+v", got)
	}
}

func TestOptionsTreatsVerticalVideoByShortSide(t *testing.T) {
	meta := parseSample(t, `{
      "id": "short1", "title": "s", "duration": 30,
      "formats": [
        {"format_id": "140", "vcodec": "none", "acodec": "mp4a.40.2", "abr": 129.0, "filesize": 500000},
        {"format_id": "137", "vcodec": "avc1.640028", "acodec": "none", "width": 1080, "height": 1920, "filesize": 9000000},
        {"format_id": "135", "vcodec": "avc1.4d401e", "acodec": "none", "width": 480,  "height": 854,  "filesize": 1000000}
      ]
    }`)

	opts := Options(meta, DefaultHeights())
	if len(opts) != 2 || opts[0].Height != 480 || opts[1].Height != 1080 {
		t.Fatalf("vertical video should be offered by short side, got %+v", opts)
	}
}

func TestOptionsEstimatesSizeFromBitrate(t *testing.T) {
	meta := parseSample(t, `{
      "id": "nosize", "title": "s", "duration": 100,
      "formats": [
        {"format_id": "140", "vcodec": "none", "acodec": "mp4a.40.2", "tbr": 128.0},
        {"format_id": "137", "vcodec": "avc1.640028", "acodec": "none", "width": 1920, "height": 1080, "tbr": 4000.0}
      ]
    }`)

	opts := Options(meta, []int{1080})
	if len(opts) != 1 {
		t.Fatalf("expected one option, got %+v", opts)
	}
	// (4000 + 128) kbit/s * 100 s / 8 = 51_600_000 bytes
	if opts[0].SizeBytes != 51600000 {
		t.Fatalf("unexpected estimate: %d", opts[0].SizeBytes)
	}
}

func TestOptionsWithoutMetadata(t *testing.T) {
	if opts := Options(nil, DefaultHeights()); opts != nil {
		t.Fatalf("expected no options, got %+v", opts)
	}
}

func TestAllowedHeightsSortsAndFallsBack(t *testing.T) {
	got := AllowedHeights([]string{"720", " 144 ", "1080", "720", "junk"})
	want := []int{144, 720, 1080}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if len(AllowedHeights(nil)) != len(DefaultHeights()) {
		t.Fatalf("empty input should fall back to defaults")
	}
}
