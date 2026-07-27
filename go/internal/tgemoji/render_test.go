package tgemoji

import (
	"strings"
	"testing"
)

func TestRenderEscapesBeforeDecorating(t *testing.T) {
	got := Render(`<b>Rick & Morty</b>`)
	if strings.Contains(got, "<b>") {
		t.Fatalf("markup must be escaped, got %q", got)
	}
	if !strings.Contains(got, "&amp;") || !strings.Contains(got, "&lt;b&gt;") {
		t.Fatalf("unexpected escaping: %q", got)
	}
}

func TestRenderUpgradesKnownEmoji(t *testing.T) {
	got := Render("👤 channel")
	if !strings.HasPrefix(got, `<tg-emoji emoji-id="`) || !strings.Contains(got, "</tg-emoji> channel") {
		t.Fatalf("emoji not upgraded: %q", got)
	}
	// The tag keeps the plain character as the non-premium fallback.
	if !strings.Contains(got, ">👤<") {
		t.Fatalf("fallback character lost: %q", got)
	}
}

func TestRenderIsStableForTheSameCharacter(t *testing.T) {
	if Render("👤") != Render("👤") {
		t.Fatal("the same character must always render as the same icon")
	}
}

func TestRenderLeavesUncoveredEmojiAlone(t *testing.T) {
	// ♫ is not part of the pack; it must survive as a plain character.
	if got := Render("♫ Mp3"); got != "♫ Mp3" {
		t.Fatalf("uncovered emoji should be untouched, got %q", got)
	}
}

func TestRenderPrefersTheVariationSelectorForm(t *testing.T) {
	// "⚙️" and "⚙" are both in the pack; the longer key must win so the
	// selector is not left stranded outside the tag.
	got := Render("⚙️")
	if strings.HasSuffix(got, "️") {
		t.Fatalf("variation selector escaped the tag: %q", got)
	}
	if strings.Count(got, "<tg-emoji") != 1 {
		t.Fatalf("expected exactly one tag, got %q", got)
	}
}

func TestRenderEmpty(t *testing.T) {
	if Render("") != "" {
		t.Fatal("empty input must stay empty")
	}
}

func TestCovers(t *testing.T) {
	if !Covers("👤") {
		t.Fatal("👤 should be covered by the pack")
	}
	if Covers("♫") {
		t.Fatal("♫ is not in the pack")
	}
}
