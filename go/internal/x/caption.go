package x

import (
	"context"
	"strings"

	"saveinator/internal/xphotos"
)

// ResolveTitle prefers tweet metadata (author/text) and falls back to cleaning the yt-dlp filename.
func ResolveTitle(ctx context.Context, statusID, videoPath string) string {
	if statusID != "" {
		if meta, err := xphotos.FetchTweetMeta(ctx, statusID); err == nil {
			if title := formatTweetTitle(meta); title != "" {
				return title
			}
		}
	}
	return DisplayTitle(videoPath)
}

func formatTweetTitle(meta *xphotos.TweetMeta) string {
	if meta == nil {
		return ""
	}
	text := CleanRawTitle(meta.Text)
	author := strings.TrimSpace(meta.Author)
	if text != "" {
		return text
	}
	return author
}
