package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Format is the subset of a yt-dlp format entry we need to size and pick a
// download.
type Format struct {
	FormatID       string  `json:"format_id"`
	Ext            string  `json:"ext"`
	VCodec         string  `json:"vcodec"`
	ACodec         string  `json:"acodec"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Filesize       int64   `json:"filesize"`
	FilesizeApprox int64   `json:"filesize_approx"`
	TBR            float64 `json:"tbr"`
	ABR            float64 `json:"abr"`
}

// Meta is the subset of `yt-dlp -J` output the format card is built from.
type Meta struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Uploader string   `json:"uploader"`
	Channel  string   `json:"channel"`
	Duration float64  `json:"duration"`
	Formats  []Format `json:"formats"`
}

// Option is one downloadable variant offered on the format card.
type Option struct {
	// Height is the short side of the frame, so a 1080x1920 vertical video
	// is offered as 1080p just like a 1920x1080 landscape one.
	Height int `json:"height"`
	// SizeBytes is an estimate: yt-dlp's own reported size when available,
	// otherwise derived from bitrate and duration.
	SizeBytes int64 `json:"size_bytes"`
	// FormatID is the exact yt-dlp selector for this option ("137+140" or a
	// single progressive id), so the download matches the advertised size.
	FormatID string `json:"format_id"`
}

func (m *Meta) Author() string {
	if strings.TrimSpace(m.Uploader) != "" {
		return m.Uploader
	}
	return m.Channel
}

func (m *Meta) DurationSec() int {
	if m.Duration <= 0 {
		return 0
	}
	return int(m.Duration + 0.5)
}

// Probe fetches video metadata without downloading anything.
func Probe(ctx context.Context, url string, timeout time.Duration) (*Meta, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "yt-dlp",
		"-J", "--no-playlist", "--quiet", url,
	).Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp probe failed: %w", withStderr(err))
	}
	return ParseMeta(out)
}

// withStderr surfaces what yt-dlp printed. Output() parks stderr on the
// ExitError, where the default formatting shows only "exit status 1" — so a
// probe that failed on a bot check, a PO token or a captcha reaches the log
// indistinguishable from any other failure.
func withStderr(err error) error {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return err
	}
	detail := strings.TrimSpace(string(exitErr.Stderr))
	if detail == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, detail)
}

func ParseMeta(data []byte) (*Meta, error) {
	var meta Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parse yt-dlp metadata: %w", err)
	}
	return &meta, nil
}

// AllowedHeights turns the runtime "youtube.allowed_qualities" list into a
// sorted, de-duplicated set of heights.
func AllowedHeights(values []string) []int {
	seen := make(map[int]struct{}, len(values))
	var out []int
	for _, v := range values {
		h, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || h <= 0 {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	if len(out) == 0 {
		return DefaultHeights()
	}
	sort.Ints(out)
	return out
}

func DefaultHeights() []int {
	return []int{144, 240, 360, 480, 720, 1080}
}

// Options lists the qualities this video actually offers, in ascending order.
// A quality is only offered when the video has a format at exactly that
// resolution, so a 720p upload never shows a 1080p button that would silently
// deliver 720p.
func Options(meta *Meta, allowed []int) []Option {
	if meta == nil {
		return nil
	}
	audio := bestAudio(meta.Formats)
	byHeight := make(map[int]*Format)
	for i := range meta.Formats {
		f := &meta.Formats[i]
		if !hasVideo(f) {
			continue
		}
		h := shortSide(f)
		if h == 0 {
			continue
		}
		if cur := byHeight[h]; cur == nil || betterVideo(f, cur, meta.Duration) {
			byHeight[h] = f
		}
	}

	var out []Option
	for _, h := range allowed {
		f := byHeight[h]
		if f == nil {
			continue
		}
		size := formatSize(f, meta.Duration)
		if hasAudio(f) {
			out = append(out, Option{Height: h, SizeBytes: size, FormatID: f.FormatID})
			continue
		}
		if audio == nil {
			continue
		}
		out = append(out, Option{
			Height:    h,
			SizeBytes: size + formatSize(audio, meta.Duration),
			FormatID:  f.FormatID + "+" + audio.FormatID,
		})
	}
	return out
}

func hasVideo(f *Format) bool {
	return f.VCodec != "" && f.VCodec != "none"
}

func hasAudio(f *Format) bool {
	return f.ACodec != "" && f.ACodec != "none"
}

func shortSide(f *Format) int {
	if f.Width > 0 && f.Height > 0 && f.Width < f.Height {
		return f.Width
	}
	return f.Height
}

// videoCodecRank prefers codecs that merge into a plain mp4. Telegram renders
// h264/aac inline with a thumbnail and correct dimensions; vp9 or av01 force a
// webm/mkv container that several clients refuse to play inline.
func videoCodecRank(codec string) int {
	c := strings.ToLower(codec)
	switch {
	case strings.HasPrefix(c, "avc1"), strings.HasPrefix(c, "h264"):
		return 0
	case strings.HasPrefix(c, "vp9"), strings.HasPrefix(c, "vp09"):
		return 1
	case strings.HasPrefix(c, "av01"):
		return 2
	default:
		return 3
	}
}

func audioCodecRank(codec string) int {
	c := strings.ToLower(codec)
	switch {
	case strings.HasPrefix(c, "mp4a"), strings.HasPrefix(c, "aac"):
		return 0
	case strings.HasPrefix(c, "opus"):
		return 1
	default:
		return 2
	}
}

// betterVideo reports whether a should replace b as the pick for their shared
// resolution: a self-contained progressive format first (no merge step at all),
// then the preferred container codec, then the smaller file.
func betterVideo(a, b *Format, duration float64) bool {
	if hasAudio(a) != hasAudio(b) {
		return hasAudio(a)
	}
	ra, rb := videoCodecRank(a.VCodec), videoCodecRank(b.VCodec)
	if ra != rb {
		return ra < rb
	}
	sa, sb := formatSize(a, duration), formatSize(b, duration)
	if sa == 0 || sb == 0 {
		return sb == 0 && sa > 0
	}
	return sa < sb
}

// bestAudio picks the audio track a merged download would carry: mp4-friendly
// codec first, highest bitrate second.
func bestAudio(formats []Format) *Format {
	var best *Format
	for i := range formats {
		f := &formats[i]
		if hasVideo(f) || !hasAudio(f) {
			continue
		}
		if best == nil {
			best = f
			continue
		}
		ra, rb := audioCodecRank(f.ACodec), audioCodecRank(best.ACodec)
		if ra != rb {
			if ra < rb {
				best = f
			}
			continue
		}
		if audioBitrate(f) > audioBitrate(best) {
			best = f
		}
	}
	return best
}

func audioBitrate(f *Format) float64 {
	if f.ABR > 0 {
		return f.ABR
	}
	return f.TBR
}

func formatSize(f *Format, duration float64) int64 {
	if f == nil {
		return 0
	}
	if f.Filesize > 0 {
		return f.Filesize
	}
	if f.FilesizeApprox > 0 {
		return f.FilesizeApprox
	}
	if f.TBR > 0 && duration > 0 {
		return int64(f.TBR * 1000 * duration / 8)
	}
	return 0
}
