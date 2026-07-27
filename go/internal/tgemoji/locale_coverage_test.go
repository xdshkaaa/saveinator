package tgemoji

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// buttonKeys are locale strings used as inline keyboard labels. Telegram
// carries no entities in button text, so their emoji must stay plain and are
// exempt from pack coverage.
var buttonKeys = map[string]bool{
	"admin.confirm_yes":          true,
	"admin.confirm_no":           true,
	"broadcast.audience_all":     true,
	"broadcast.audience_active":  true,
	"broadcast.audience_test":    true,
	"onboarding.btn_en":          true,
	"onboarding.btn_ru":          true,
	"onboarding.btn_kk":          true,
	"youtube.btn_quality":        true,
	"youtube.btn_mp3":            true,
	"youtube.btn_trim":           true,
	"youtube.btn_trim_cancel":    true,
	"download.queue_button":      true,
	"tiktok.btn_carousel_images": true,
}

// TestLocaleEmojiAreCovered keeps message bodies visually consistent: every
// emoji a user sees in a message must exist in the pack, otherwise one message
// mixes premium icons with plain system ones. Button labels are exempt.
func TestLocaleEmojiAreCovered(t *testing.T) {
	dir := filepath.Join("..", "locale", "locales")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read locales: %v", err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		var tree map[string]any
		if err := json.Unmarshal(raw, &tree); err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		walkLocale(t, entry.Name(), tree, "")
	}
}

func walkLocale(t *testing.T, file string, node map[string]any, prefix string) {
	t.Helper()
	for k, v := range node {
		key := prefix + k
		switch typed := v.(type) {
		case map[string]any:
			walkLocale(t, file, typed, key+".")
		case string:
			if strings.Contains(typed, "<tg-emoji") {
				t.Errorf("%s: %s carries a raw <tg-emoji> tag; tags are applied by Render at send time", file, key)
			}
			if strings.Contains(key, "btn_") || buttonKeys[key] {
				continue
			}
			for _, r := range typed {
				if !isPictograph(r) {
					continue
				}
				if !Covers(string(r)) && !Covers(string(r)+"️") {
					t.Errorf("%s: %s uses %q, which the pack does not cover — pick a covered icon", file, key, string(r))
				}
			}
		}
	}
}

// isPictograph reports whether r is an emoji-like symbol we expect the pack to
// cover. Flags (regional indicators) are excluded: they are language buttons.
func isPictograph(r rune) bool {
	if r < 0x2000 || unicode.IsLetter(r) || unicode.IsDigit(r) {
		return false
	}
	switch {
	case r >= 0x1F1E6 && r <= 0x1F1FF: // regional indicators
		return false
	case r == 0xFE0F || r == 0x20E3: // variation selector, keycap
		return false
	case r >= 0x2010 && r <= 0x2027: // dashes, quotes, ellipsis
		return false
	case r >= 0x2030 && r <= 0x205F: // punctuation
		return false
	}
	return r >= 0x2190 && r <= 0x2BFF || r >= 0x1F300 && r <= 0x1FAFF
}
