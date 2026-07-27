package handler

import (
	"errors"
	"testing"

	"saveinator/internal/queue"
	"saveinator/internal/youtube"
)

func testSession() youtube.PendingSession {
	return youtube.PendingSession{
		UserID:    42,
		ChatID:    7,
		MessageID: 11,
		Lang:      "ru",
		URL:       "https://www.youtube.com/watch?v=rhkpz9Ud4iU",
		Title:     "ЗАЙКА",
		Author:    "МЭЙБИ БЭЙБИ",
	}
}

func TestBuildYouTubePayload_video(t *testing.T) {
	p := buildYouTubePayload(testSession(), youtube.Option{Height: 1080, FormatID: "137+140"}, false, "")

	if p.Quality != 1080 || p.FormatID != "137+140" {
		t.Fatalf("unexpected video payload: %+v", p)
	}
	if p.AudioOnly || p.IsTrimmed() {
		t.Fatalf("plain download should be neither audio nor trimmed: %+v", p)
	}
	if p.AspectRatio != "" {
		t.Fatalf("original frame should not request a transcode: %q", p.AspectRatio)
	}
}

func TestBuildYouTubePayload_fallsBackToGenericSelector(t *testing.T) {
	p := buildYouTubePayload(testSession(), youtube.Option{Height: 720}, false, "9_16")
	if p.FormatID != youtube.BuildFormat(720, "9_16") {
		t.Fatalf("expected generic selector, got %q", p.FormatID)
	}
}

func TestBuildYouTubePayload_audioOnly(t *testing.T) {
	p := buildYouTubePayload(testSession(), youtube.Option{Height: 1080, FormatID: "137+140"}, true, "16_9")

	if !p.AudioOnly {
		t.Fatal("expected an audio-only payload")
	}
	if p.Quality != 0 || p.FormatID != "" || p.AspectRatio != "" {
		t.Fatalf("audio payload must carry no video settings: %+v", p)
	}
	if p.Title != "ЗАЙКА" || p.Author != "МЭЙБИ БЭЙБИ" {
		t.Fatalf("audio payload must carry tags: %+v", p)
	}
}

func TestBuildYouTubePayload_carriesFragment(t *testing.T) {
	session := testSession()
	session.TrimStart, session.TrimEnd = 80, 165

	p := buildYouTubePayload(session, youtube.Option{Height: 720, FormatID: "136+140"}, false, "")
	if !p.IsTrimmed() || p.TrimStart != 80 || p.TrimEnd != 165 {
		t.Fatalf("unexpected fragment: %+v", p)
	}
}

func TestTrimErrorText(t *testing.T) {
	cases := map[error]string{
		youtube.ErrTrimOrder:  "youtube.trim_order",
		youtube.ErrTrimRange:  "youtube.trim_out_of_range",
		youtube.ErrTrimFormat: "youtube.trim_invalid",
		errors.New("other"):   "youtube.trim_invalid",
	}
	for err, key := range cases {
		got := trimErrorText(err, "en")
		if got == "" || got == key {
			t.Fatalf("%v: expected a localised string, got %q", err, got)
		}
	}
	if trimErrorText(youtube.ErrTrimOrder, "en") == trimErrorText(youtube.ErrTrimFormat, "en") {
		t.Fatal("distinct trim errors should produce distinct messages")
	}
}

func TestIsTranscodePayloadRouting(t *testing.T) {
	// An explicit aspect ratio is the only thing that re-encodes video.
	transcode := buildYouTubePayload(testSession(), youtube.Option{Height: 1080}, false, "16_9")
	original := buildYouTubePayload(testSession(), youtube.Option{Height: 1080}, false, "")
	audio := buildYouTubePayload(testSession(), youtube.Option{}, true, "")

	if queue.QueueFor(transcode) != queue.QueueTranscode {
		t.Fatal("aspect-ratio job belongs on the transcode queue")
	}
	if queue.QueueFor(original) != queue.QueueDownload {
		t.Fatal("original-frame job must not occupy the transcode queue")
	}
	if queue.QueueFor(audio) != queue.QueueDownload {
		t.Fatal("audio job must not occupy the transcode queue")
	}
}
