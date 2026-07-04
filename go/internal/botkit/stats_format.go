package botkit

import (
	"fmt"
	"strconv"

	"saveinator/internal/locale"
)

func formatPct(part, total int) string {
	if total <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d%%", 100*part/total)
}

func formatStickiness(dau, mau int) string {
	if mau <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d%%", 100*dau/mau)
}

func formatSuccessRate(completed, failed int) string {
	total := completed + failed
	if total <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d%% (%d/%d)", 100*completed/total, completed, total)
}

func formatGrowthDelta(today, yesterday int, lang string) string {
	diff := today - yesterday
	if diff == 0 {
		return " " + locale.Get("admin.stats_growth_same", lang, nil)
	}
	pct := "new"
	if yesterday > 0 {
		pct = fmt.Sprintf("%+d%%", 100*diff/yesterday)
	}
	return locale.Get("admin.stats_growth_delta", lang, map[string]string{
		"diff": fmt.Sprintf("%+d", diff),
		"pct":  pct,
	})
}

func formatStatsDownloads(today, d7, d30 int) string {
	return strconv.Itoa(today) + " · 7d: " + strconv.Itoa(d7) + " · 30d: " + strconv.Itoa(d30)
}

func formatStatsDownloadsRU(today, d7, d30 int) string {
	return strconv.Itoa(today) + " · 7д: " + strconv.Itoa(d7) + " · 30д: " + strconv.Itoa(d30)
}
