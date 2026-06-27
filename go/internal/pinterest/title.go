package pinterest

import (
	"path/filepath"
	"strings"
)

// DisplayTitle extracts a human-readable title from a Pinterest download filename.
// Pin files are saved as pin_{id}.{ext}; board items as board_{id}_{type}.{ext}.
func DisplayTitle(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	if strings.HasPrefix(base, "pin_") && isPinID(strings.TrimPrefix(base, "pin_")) {
		return ""
	}
	if strings.HasPrefix(base, "board_") {
		rest := strings.TrimPrefix(base, "board_")
		if idx := strings.Index(rest, "_"); idx > 0 {
			idPart := rest[:idx]
			typePart := rest[idx+1:]
			if isPinID(idPart) && (typePart == "image" || typePart == "video") {
				return ""
			}
		}
	}
	return base
}

func isPinID(s string) bool {
	if len(s) < 10 || len(s) > 20 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
