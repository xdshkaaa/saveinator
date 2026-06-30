package locale

import "testing"

func TestGetEnglish(t *testing.T) {
	t.Parallel()
	got := Get("download.cancel", "en", nil)
	if got != "Cancel" {
		t.Fatalf("Get = %q, want Cancel", got)
	}
}

func TestGetRussian(t *testing.T) {
	t.Parallel()
	got := Get("download.cancel", "ru", nil)
	if got != "Отмена" {
		t.Fatalf("Get = %q, want Отмена", got)
	}
}

func TestGetFallbackToEnglish(t *testing.T) {
	t.Parallel()
	got := Get("download.cancel", "xx", nil)
	if got != "Cancel" {
		t.Fatalf("fallback Get = %q, want Cancel", got)
	}
}

func TestGetMissingKey(t *testing.T) {
	t.Parallel()
	got := Get("does.not.exist", "en", nil)
	if got != "does.not.exist" {
		t.Fatalf("Get = %q, want key returned", got)
	}
}

func TestGetVariableSubstitution(t *testing.T) {
	t.Parallel()
	got := Get("download.queue_remove", "en", map[string]string{"item": "TikTok"})
	if got != "Remove TikTok download" {
		t.Fatalf("Get = %q, want substituted text", got)
	}
}
