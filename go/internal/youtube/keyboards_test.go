package youtube

import "testing"

func TestBuildFormat(t *testing.T) {
	format := BuildFormat(1080, "16_9")
	if format == "" || !contains(format, "height<=1080") {
		t.Fatalf("unexpected format: %s", format)
	}
	format = BuildFormat(720, "9_16")
	if !contains(format, "width<=720") {
		t.Fatalf("unexpected format: %s", format)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || index(s, sub) >= 0)
}

func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
