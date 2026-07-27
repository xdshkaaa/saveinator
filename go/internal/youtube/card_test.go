package youtube

import (
	"strings"
	"testing"
)

func TestCardListsMetadataAndSizes(t *testing.T) {
	meta := parseSample(t, sampleMeta)
	card := Card("ru", meta, Options(meta, DefaultHeights()), "")

	for _, want := range []string{
		"ЗАЙКА (lyric video)",
		"МЭЙБИ БЭЙБИ",
		"2:31",
		"360p: 5 MB",
		"1080p: 97 MB",
		"Выберите формат скачивания",
	} {
		if !strings.Contains(card, want) {
			t.Fatalf("card missing %q:\n%s", want, card)
		}
	}
}

func TestCardShowsSelectedFragment(t *testing.T) {
	meta := parseSample(t, sampleMeta)
	card := Card("ru", meta, Options(meta, DefaultHeights()), FormatRange(80, 165))
	if !strings.Contains(card, "1:20–2:45") {
		t.Fatalf("card missing fragment:\n%s", card)
	}
}

func TestCardSurvivesMissingMetadata(t *testing.T) {
	card := Card("en", nil, []Option{{Height: 720}}, "")
	if !strings.Contains(card, "Choose a download format") {
		t.Fatalf("card should still prompt for a format:\n%s", card)
	}
	// A quality with no size estimate must not print an empty size line.
	if strings.Contains(card, "720p:") {
		t.Fatalf("card should omit unknown sizes:\n%s", card)
	}
}
