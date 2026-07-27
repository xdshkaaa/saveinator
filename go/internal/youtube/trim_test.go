package youtube

import (
	"errors"
	"testing"
)

func TestParseRangeAcceptedForms(t *testing.T) {
	cases := []struct {
		in                 string
		duration           int
		wantStart, wantEnd float64
	}{
		{"1:20-2:45", 300, 80, 165},
		{"01:20 - 02:45", 300, 80, 165},
		{"0:00:30-0:01:00", 300, 30, 60},
		{"90-165", 300, 90, 165},
		{"1:00—2:00", 300, 60, 120},
		{"0:10-0:20.5", 300, 10, 20.5},
		{"0:00-9:59", 0, 0, 599},
		// The end is clamped to the video length instead of rejected.
		{"1:00-99:00", 151, 60, 151},
	}
	for _, c := range cases {
		start, end, err := ParseRange(c.in, c.duration)
		if err != nil {
			t.Fatalf("ParseRange(%q): %v", c.in, err)
		}
		if start != c.wantStart || end != c.wantEnd {
			t.Fatalf("ParseRange(%q) = %v-%v, want %v-%v", c.in, start, end, c.wantStart, c.wantEnd)
		}
	}
}

func TestParseRangeRejections(t *testing.T) {
	cases := []struct {
		in       string
		duration int
		want     error
	}{
		{"", 300, ErrTrimFormat},
		{"1:20", 300, ErrTrimFormat},
		{"1:20-2:45-3:00", 300, ErrTrimFormat},
		{"abc-def", 300, ErrTrimFormat},
		{"1:2:3:4-5:00", 300, ErrTrimFormat},
		{"2:45-1:20", 300, ErrTrimOrder},
		{"1:20-1:20", 300, ErrTrimOrder},
		{"5:00-6:00", 151, ErrTrimRange},
	}
	for _, c := range cases {
		if _, _, err := ParseRange(c.in, c.duration); !errors.Is(err, c.want) {
			t.Fatalf("ParseRange(%q) = %v, want %v", c.in, err, c.want)
		}
	}
}

func TestDownloadSection(t *testing.T) {
	if got := DownloadSection(80, 165); got != "*80.00-165.00" {
		t.Fatalf("unexpected section: %s", got)
	}
}

func TestFormatDurationAndSize(t *testing.T) {
	if got := FormatDuration(151); got != "2:31" {
		t.Fatalf("duration: %s", got)
	}
	if got := FormatDuration(3725); got != "1:02:05" {
		t.Fatalf("duration: %s", got)
	}
	if got := FormatSize(102 * 1024 * 1024); got != "102 MB" {
		t.Fatalf("size: %s", got)
	}
	if got := FormatSize(400 * 1024); got != "400 KB" {
		t.Fatalf("size: %s", got)
	}
}
