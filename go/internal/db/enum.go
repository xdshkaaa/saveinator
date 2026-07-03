package db

import "strings"

func toDBLanguage(lang string) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "ru":
		return "RU"
	case "kk":
		return "KK"
	default:
		return "EN"
	}
}

func fromDBLanguage(lang string) string {
	switch strings.ToUpper(strings.TrimSpace(lang)) {
	case "RU":
		return "ru"
	case "KK":
		return "kk"
	default:
		return "en"
	}
}

func toDBPlatform(platform string) string {
	return strings.ToUpper(strings.TrimSpace(platform))
}

func FromDBPlatform(platform string) string {
	return strings.ToLower(strings.TrimSpace(platform))
}

func toDBDownloadStatus(status string) string {
	return strings.ToUpper(strings.TrimSpace(status))
}
