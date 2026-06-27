package pinterest

import "errors"

var (
	ErrInvalidURL = errors.New("invalid pinterest url")
	ErrNoMedia    = errors.New("no pinterest media found")
	ErrDownload   = errors.New("pinterest download failed")
)

type MediaItem struct {
	Title     string
	MediaType string // image | video
	FilePath  string
	FileSize  int64
}

type DownloadResult struct {
	URL     string
	URLType URLType
	Items   []MediaItem
}
