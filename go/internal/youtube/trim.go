package youtube

import (
	"errors"
	"strconv"
	"strings"
)

var (
	// ErrTrimFormat means the text is not a time range at all.
	ErrTrimFormat = errors.New("youtube: unrecognised time range")
	// ErrTrimOrder means the range ends at or before it starts.
	ErrTrimOrder = errors.New("youtube: trim end must be after start")
	// ErrTrimRange means the range starts past the end of the video.
	ErrTrimRange = errors.New("youtube: trim start is past the video end")
)

// ParseRange reads a user-typed fragment such as "1:20-2:45", "01:20 - 02:45",
// "0:00:30-0:01:00" or plain seconds "90-165". When durationSec is known the
// end is clamped to it, so "1:00-99:00" on a 2-minute video means "to the end".
func ParseRange(input string, durationSec int) (float64, float64, error) {
	normalized := strings.NewReplacer("—", "-", "–", "-", "−", "-", "..", "-").Replace(input)
	parts := strings.Split(normalized, "-")
	if len(parts) != 2 {
		return 0, 0, ErrTrimFormat
	}

	start, err := parseTimecode(parts[0])
	if err != nil {
		return 0, 0, err
	}
	end, err := parseTimecode(parts[1])
	if err != nil {
		return 0, 0, err
	}

	if durationSec > 0 {
		if start >= float64(durationSec) {
			return 0, 0, ErrTrimRange
		}
		if end > float64(durationSec) {
			end = float64(durationSec)
		}
	}
	if end <= start {
		return 0, 0, ErrTrimOrder
	}
	return start, end, nil
}

func parseTimecode(raw string) (float64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, ErrTrimFormat
	}
	fields := strings.Split(s, ":")
	if len(fields) > 3 {
		return 0, ErrTrimFormat
	}
	total := 0.0
	for i, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			return 0, ErrTrimFormat
		}
		// Only the last field may be fractional; the rest are whole units.
		if i == len(fields)-1 {
			v, err := strconv.ParseFloat(field, 64)
			if err != nil || v < 0 {
				return 0, ErrTrimFormat
			}
			total += v
			continue
		}
		v, err := strconv.Atoi(field)
		if err != nil || v < 0 {
			return 0, ErrTrimFormat
		}
		total += float64(v) * unitSeconds(len(fields), i)
	}
	return total, nil
}

func unitSeconds(fieldCount, index int) float64 {
	// Rightmost field is seconds, so its weight is 60^0, the next 60^1, etc.
	power := fieldCount - 1 - index
	weight := 1.0
	for i := 0; i < power; i++ {
		weight *= 60
	}
	return weight
}

// FormatRange renders a parsed fragment back for display.
func FormatRange(start, end float64) string {
	return FormatDuration(int(start)) + "–" + FormatDuration(int(end+0.5))
}

// DownloadSection renders a range as a yt-dlp --download-sections argument.
func DownloadSection(start, end float64) string {
	return "*" + strconv.FormatFloat(start, 'f', 2, 64) + "-" + strconv.FormatFloat(end, 'f', 2, 64)
}
