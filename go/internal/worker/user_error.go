package worker

import (
	"strings"

	"saveinator/internal/locale"
	"saveinator/internal/ytdlp"
)

const maxAdminDebugLen = 3500

func (h *Handler) isAdmin(userID int64) bool {
	return h.cfg.AdminTelegramID != 0 && userID == h.cfg.AdminTelegramID
}

// userFacingError returns a message for regular users, classified from the
// underlying error where a known pattern matches (see ytdlp.UserFacingErrorKey),
// falling back to a generic message otherwise. Admins also get the underlying
// execution error to simplify production debugging.
func (h *Handler) userFacingError(lang string, userID int64, err error) string {
	key := ytdlp.UserFacingErrorKey("", err)
	if key == "" {
		key = "errors.generic"
	}
	base := locale.Get(key, lang, nil)
	if !h.isAdmin(userID) || err == nil {
		return base
	}
	detail := strings.TrimSpace(err.Error())
	if detail == "" {
		return base
	}
	if len(detail) > maxAdminDebugLen {
		detail = detail[:maxAdminDebugLen] + "…"
	}
	return base + locale.Get("errors.admin_debug", lang, map[string]string{"detail": detail})
}
