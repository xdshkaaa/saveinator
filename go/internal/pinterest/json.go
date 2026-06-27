package pinterest

import "time"

type MediaItemJSON struct {
	SourceURL        string `json:"source_url"`
	MediaType        string `json:"media_type"`
	Title            string `json:"title,omitempty"`
	Description      string `json:"description,omitempty"`
	OriginalMediaURL string `json:"original_media_url,omitempty"`
	FilePath         string `json:"file_path"`
	FileSize         int64  `json:"file_size"`
	CreatedAt        string `json:"created_at"`
}

type DownloadResultJSON struct {
	URL     string          `json:"url"`
	URLType string          `json:"url_type"`
	Items   []MediaItemJSON `json:"items"`
	Errors  []string        `json:"errors"`
	Count   int             `json:"count"`
}

func (r *DownloadResult) ToJSON() DownloadResultJSON {
	items := make([]MediaItemJSON, 0, len(r.Items))
	now := time.Now().UTC().Format(time.RFC3339) + "Z"
	for _, item := range r.Items {
		items = append(items, MediaItemJSON{
			SourceURL: r.URL,
			MediaType: item.MediaType,
			Title:     item.Title,
			FilePath:  item.FilePath,
			FileSize:  item.FileSize,
			CreatedAt: now,
		})
	}
	return DownloadResultJSON{
		URL:     r.URL,
		URLType: string(r.URLType),
		Items:   items,
		Errors:  nil,
		Count:   len(items),
	}
}
