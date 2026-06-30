package worker

import (
	"errors"
	"strings"
	"testing"

	"saveinator/internal/config"
)

func TestUserFacingError_regularUser(t *testing.T) {
	t.Parallel()
	h := &Handler{cfg: &config.Settings{AdminTelegramID: 42}}
	got := h.userFacingError("en", 7, errors.New("yt-dlp failed: secret details"))
	if strings.Contains(got, "secret details") {
		t.Fatalf("regular user should not see debug details: %q", got)
	}
}

func TestUserFacingError_adminGetsDebug(t *testing.T) {
	t.Parallel()
	h := &Handler{cfg: &config.Settings{AdminTelegramID: 42}}
	got := h.userFacingError("en", 42, errors.New("yt-dlp failed: secret details"))
	if !strings.Contains(got, "secret details") {
		t.Fatalf("admin should see debug details: %q", got)
	}
}

func TestUserFacingError_adminWithoutError(t *testing.T) {
	t.Parallel()
	h := &Handler{cfg: &config.Settings{AdminTelegramID: 42}}
	got := h.userFacingError("en", 42, nil)
	if strings.Contains(got, "Debug") || strings.Contains(got, "Отладка") {
		t.Fatalf("admin without error should not get debug block: %q", got)
	}
}
