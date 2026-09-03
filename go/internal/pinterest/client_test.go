package pinterest

import "testing"

func videoList(entries map[string]any) map[string]any {
	return entries
}

func TestBestVideoURLPrefersMP4OverHLS(t *testing.T) {
	// Real video pins expose mp4 and HLS variants with the same width; the
	// mp4 must win or the download would need ffmpeg remuxing.
	list := videoList(map[string]any{
		"V_720P":         map[string]any{"url": "https://v1.pinimg.com/videos/a_720w.mp4", "width": float64(720)},
		"V_HLSV4":        map[string]any{"url": "https://v1.pinimg.com/videos/a.m3u8", "width": float64(720)},
		"V_HLSV3_MOBILE": map[string]any{"url": "https://v1.pinimg.com/videos/b.m3u8", "width": float64(720)},
	})
	url, ok := bestVideoURL(list)
	if !ok || url != "https://v1.pinimg.com/videos/a_720w.mp4" {
		t.Fatalf("got %q ok=%v, want mp4 url", url, ok)
	}
}

func TestBestVideoURLHighestWidthMP4(t *testing.T) {
	list := videoList(map[string]any{
		"V_720P":  map[string]any{"url": "https://v1.pinimg.com/videos/a_720w.mp4", "width": float64(720)},
		"V_1080P": map[string]any{"url": "https://v1.pinimg.com/videos/a_1080w.mp4", "width": float64(1080)},
	})
	url, _ := bestVideoURL(list)
	if url != "https://v1.pinimg.com/videos/a_1080w.mp4" {
		t.Fatalf("got %q, want 1080p url", url)
	}
}

func TestBestVideoURLHLSFallback(t *testing.T) {
	// Idea pins ship HLS only; the HLS variant must be returned instead of nothing.
	list := videoList(map[string]any{
		"V_HLSV3_MOBILE": map[string]any{"url": "https://v1.pinimg.com/videos/a.m3u8", "width": float64(720)},
		"V_HLSV4":        map[string]any{"url": "https://v1.pinimg.com/videos/b.m3u8", "width": float64(1080)},
	})
	url, ok := bestVideoURL(list)
	if !ok || url != "https://v1.pinimg.com/videos/b.m3u8" {
		t.Fatalf("got %q ok=%v, want widest HLS url", url, ok)
	}
}

func TestBestVideoURLEmpty(t *testing.T) {
	if _, ok := bestVideoURL(videoList(map[string]any{
		"V_720P": map[string]any{"url": "", "width": float64(720)},
	})); ok {
		t.Fatal("expected no url for empty entries")
	}
}

func TestPinMediaStoryPinVideo(t *testing.T) {
	// Idea pins carry video only under story_pin_data; pinMedia must find it
	// and report media type "video" instead of falling back to the cover image.
	data := map[string]any{
		"grid_title": "some pin",
		"images":     map[string]any{"orig": map[string]any{"url": "https://i.pinimg.com/cover.jpg"}},
		"story_pin_data": map[string]any{
			"pages": []any{
				map[string]any{
					"blocks": []any{
						map[string]any{
							"block_type": "video",
							"video": map[string]any{
								"video_list": map[string]any{
									"V_HLSV3_MOBILE": map[string]any{"url": "https://v1.pinimg.com/videos/a.m3u8", "width": float64(720)},
								},
							},
						},
					},
				},
			},
		},
	}
	url, mediaType, ok := pinMedia(data)
	if !ok || mediaType != "video" || url != "https://v1.pinimg.com/videos/a.m3u8" {
		t.Fatalf("got url=%q type=%q ok=%v, want story pin video", url, mediaType, ok)
	}
}

func TestPinMediaPageLevelVideo(t *testing.T) {
	data := map[string]any{
		"story_pin_data": map[string]any{
			"pages": []any{
				map[string]any{
					"video": map[string]any{
						"video_list": map[string]any{
							"V_HLSV4": map[string]any{"url": "https://v1.pinimg.com/videos/p.m3u8", "width": float64(720)},
						},
					},
				},
			},
		},
	}
	url, mediaType, ok := pinMedia(data)
	if !ok || mediaType != "video" || url != "https://v1.pinimg.com/videos/p.m3u8" {
		t.Fatalf("got url=%q type=%q ok=%v, want page-level video", url, mediaType, ok)
	}
}

func TestPinMediaImageFallback(t *testing.T) {
	data := map[string]any{
		"images": map[string]any{"orig": map[string]any{"url": "https://i.pinimg.com/cover.jpg"}},
	}
	url, mediaType, ok := pinMedia(data)
	if !ok || mediaType != "image" || url != "https://i.pinimg.com/cover.jpg" {
		t.Fatalf("got url=%q type=%q ok=%v, want image", url, mediaType, ok)
	}
}

func TestPinMediaVideoPin(t *testing.T) {
	data := map[string]any{
		"videos": map[string]any{
			"video_list": map[string]any{
				"V_720P":  map[string]any{"url": "https://v1.pinimg.com/videos/a_720w.mp4", "width": float64(720)},
				"V_HLSV4": map[string]any{"url": "https://v1.pinimg.com/videos/a.m3u8", "width": float64(720)},
			},
		},
	}
	url, mediaType, ok := pinMedia(data)
	if !ok || mediaType != "video" || url != "https://v1.pinimg.com/videos/a_720w.mp4" {
		t.Fatalf("got url=%q type=%q ok=%v, want mp4", url, mediaType, ok)
	}
}
