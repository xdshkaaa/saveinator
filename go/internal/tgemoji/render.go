package tgemoji

import (
	"html"
	"sort"
	"strings"
	"sync"
)

// Render prepares a plain-text bot message for parse_mode=HTML: it escapes the
// text, then upgrades every emoji covered by the pack to its premium custom
// emoji. Escaping first means interpolated values a user never controls the
// shape of — video titles, yt-dlp error output, broadcast bodies — can carry
// "<" or "&" without breaking the message.
//
// Only message bodies and captions may be rendered. Inline keyboard labels and
// answerCallbackQuery texts carry no entities, so a tag there would show up
// literally.
func Render(text string) string {
	if text == "" {
		return text
	}
	return Decorate(html.EscapeString(text))
}

// Decorate upgrades emoji in text that is already valid HTML. Prefer Render;
// this exists for callers that assemble markup themselves.
func Decorate(text string) string {
	if text == "" {
		return text
	}
	return replacer().Replace(text)
}

// Covers reports whether the pack has a custom emoji for a fallback character.
func Covers(fallback string) bool {
	_, ok := Emoji[fallback]
	return ok
}

var (
	replacerOnce sync.Once
	cached       *strings.Replacer
)

func replacer() *strings.Replacer {
	replacerOnce.Do(func() {
		fallbacks := make([]string, 0, len(Emoji))
		for fallback := range Emoji {
			fallbacks = append(fallbacks, fallback)
		}
		// Longest first, so "⚙️" (with the variation selector) wins over "⚙"
		// and the selector is not left dangling after the tag.
		sort.Slice(fallbacks, func(i, j int) bool {
			if len(fallbacks[i]) != len(fallbacks[j]) {
				return len(fallbacks[i]) > len(fallbacks[j])
			}
			return fallbacks[i] < fallbacks[j]
		})

		pairs := make([]string, 0, len(fallbacks)*2)
		for _, fallback := range fallbacks {
			ids := Emoji[fallback]
			if len(ids) == 0 {
				continue
			}
			// IDs are generated in a stable order; take the first so the same
			// character always renders as the same icon.
			pairs = append(pairs, fallback, TgEmoji(ids[0], fallback))
		}
		cached = strings.NewReplacer(pairs...)
	})
	return cached
}
