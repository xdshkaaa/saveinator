package worker

import (
	"strings"

	"saveinator/internal/locale"
)

const maxAdminDebugLen = 3500

func (h *Handler) isAdmin(userID int64) bool {
	return h.cfg.AdminTelegramID != 0 && userID == h.cfg.AdminTelegramID
}

// userFacingError returns a generic message for regular users. Admins also get
// the underlying execution error to simplify production debugging.
func (h *Handler) userFacingError(lang string, userID int64, err error) string {
	base := locale.Get("errors.generic", lang, nil)
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
