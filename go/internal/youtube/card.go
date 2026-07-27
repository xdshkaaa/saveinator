package youtube

import (
	"fmt"
	"strings"

	"saveinator/internal/locale"
)

// Card renders the format-picker message: what the video is, followed by every
// offered quality with its estimated size. trimLabel is empty unless the user
// has already picked a fragment.
func Card(lang string, meta *Meta, opts []Option, trimLabel string) string {
	var lines []string
	if meta != nil {
		if title := strings.TrimSpace(meta.Title); title != "" {
			lines = append(lines, locale.Get("youtube.card_title", lang, map[string]string{"title": title}))
		}
		if author := strings.TrimSpace(meta.Author()); author != "" {
			lines = append(lines, locale.Get("youtube.card_author", lang, map[string]string{"author": author}))
		}
		if sec := meta.DurationSec(); sec > 0 {
			lines = append(lines, locale.Get("youtube.card_duration", lang, map[string]string{"duration": FormatDuration(sec)}))
		}
	}
	if trimLabel != "" {
		lines = append(lines, locale.Get("youtube.card_trim", lang, map[string]string{"range": trimLabel}))
	}

	var sizes []string
	for _, opt := range opts {
		if opt.SizeBytes <= 0 {
			continue
		}
		sizes = append(sizes, locale.Get("youtube.card_size_line", lang, map[string]string{
			"quality": fmt.Sprintf("%d", opt.Height),
			"size":    FormatSize(opt.SizeBytes),
		}))
	}
	if len(sizes) > 0 {
		lines = append(lines, "", strings.Join(sizes, "\n"))
	}
	lines = append(lines, "", locale.Get("youtube.choose_format", lang, nil))
	return strings.Join(lines, "\n")
}

func FormatDuration(sec int) string {
	if sec < 0 {
		sec = 0
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func FormatSize(bytes int64) string {
	const mb = 1024 * 1024
	if bytes >= mb {
		return fmt.Sprintf("%d MB", (bytes+mb/2)/mb)
	}
	kb := (bytes + 512) / 1024
	if kb < 1 {
		kb = 1
	}
	return fmt.Sprintf("%d KB", kb)
}
