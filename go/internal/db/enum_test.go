package db

import "testing"

func TestToDBLanguage(t *testing.T) {
	tests := map[string]string{
		"en":  "EN",
		"EN":  "EN",
		"ru":  "RU",
		"RU":  "RU",
		" fr": "EN",
		"":    "EN",
	}
	for in, want := range tests {
		if got := toDBLanguage(in); got != want {
			t.Fatalf("toDBLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFromDBLanguage(t *testing.T) {
	tests := map[string]string{
		"EN": "en",
		"en": "en",
		"RU": "ru",
		"ru": "ru",
		"":   "en",
	}
	for in, want := range tests {
		if got := fromDBLanguage(in); got != want {
			t.Fatalf("fromDBLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToDBPlatform(t *testing.T) {
	if got := toDBPlatform("tiktok"); got != "TIKTOK" {
		t.Fatalf("toDBPlatform(tiktok) = %q", got)
	}
}

func TestFromDBPlatform(t *testing.T) {
	if got := FromDBPlatform("TIKTOK"); got != "tiktok" {
		t.Fatalf("FromDBPlatform(TIKTOK) = %q", got)
	}
}

func TestToDBDownloadStatus(t *testing.T) {
	if got := toDBDownloadStatus("completed"); got != "COMPLETED" {
		t.Fatalf("toDBDownloadStatus(completed) = %q", got)
	}
}
