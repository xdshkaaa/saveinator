package runtime

type ValueType string

const (
	TypeInt  ValueType = "int"
	TypeBool ValueType = "bool"
)

type Setting struct {
	RedisKey     string
	ConfigField  string
	Service      string
	ValueType    ValueType
	MinValue     int
	MaxValue     int
	LabelEN      string
	LabelRU      string
	Unit         string
}

var ServiceOrder = []string{
	"youtube", "tiktok", "instagram", "x", "spotify", "soundcloud", "pinterest",
}

var settings = []Setting{
	// global
	{RedisKey: "global.document_limit_mb", ConfigField: "SendDocumentLimitMB", Service: "global", ValueType: TypeInt, MinValue: 1, MaxValue: 1999, LabelEN: "Document limit", LabelRU: "Лимит документов", Unit: "MB"},
	{RedisKey: "global.telegram_upload_limit_mb", ConfigField: "TelegramUploadLimitMB", Service: "global", ValueType: TypeInt, MinValue: 1, MaxValue: 1999, LabelEN: "Telegram upload limit", LabelRU: "Лимит загрузки Telegram", Unit: "MB"},
	{RedisKey: "global.default_timeout_sec", ConfigField: "DownloadTimeoutSeconds", Service: "global", ValueType: TypeInt, MinValue: 10, MaxValue: 3600, LabelEN: "Default timeout", LabelRU: "Таймаут по умолчанию", Unit: "sec"},
	{RedisKey: "global.broadcast_delay_ms", ConfigField: "BroadcastDelayMS", Service: "global", ValueType: TypeInt, MinValue: 20, MaxValue: 5000, LabelEN: "Broadcast delay", LabelRU: "Задержка рассылки", Unit: "ms"},
	{RedisKey: "global.broadcast_batch_size", ConfigField: "BroadcastBatchSize", Service: "global", ValueType: TypeInt, MinValue: 1, MaxValue: 100, LabelEN: "Broadcast batch size", LabelRU: "Размер пачки рассылки"},
	// youtube
	{RedisKey: "youtube.max_file_mb", ConfigField: "YouTubeMaxFileSizeMB", Service: "youtube", ValueType: TypeInt, MinValue: 1, MaxValue: 1999, LabelEN: "Max file size", LabelRU: "Макс. размер файла", Unit: "MB"},
	{RedisKey: "youtube.timeout_sec", ConfigField: "YouTubeDownloadTimeoutSeconds", Service: "youtube", ValueType: TypeInt, MinValue: 10, MaxValue: 3600, LabelEN: "Download timeout", LabelRU: "Таймаут скачивания", Unit: "sec"},
	{RedisKey: "youtube.enabled", ConfigField: "YouTubeEnabled", Service: "youtube", ValueType: TypeBool, LabelEN: "Enabled", LabelRU: "Включено"},
	// spotify
	{RedisKey: "spotify.enabled", ConfigField: "SpotifyEnabled", Service: "spotify", ValueType: TypeBool, LabelEN: "Enabled", LabelRU: "Включено"},
	{RedisKey: "spotify.download_enabled", ConfigField: "SpotifyDownloadEnabled", Service: "spotify", ValueType: TypeBool, LabelEN: "Download enabled", LabelRU: "Скачивание включено"},
	{RedisKey: "spotify.track_timeout_sec", ConfigField: "SpotifyTrackTimeoutSeconds", Service: "spotify", ValueType: TypeInt, MinValue: 10, MaxValue: 600, LabelEN: "Track timeout", LabelRU: "Таймаут трека", Unit: "sec"},
	// soundcloud
	{RedisKey: "soundcloud.enabled", ConfigField: "SoundCloudEnabled", Service: "soundcloud", ValueType: TypeBool, LabelEN: "Enabled", LabelRU: "Включено"},
	{RedisKey: "soundcloud.download_enabled", ConfigField: "SoundCloudDownloadEnabled", Service: "soundcloud", ValueType: TypeBool, LabelEN: "Download enabled", LabelRU: "Скачивание включено"},
	{RedisKey: "soundcloud.max_tracks_per_playlist", ConfigField: "SoundCloudMaxTracks", Service: "soundcloud", ValueType: TypeInt, MinValue: 1, MaxValue: 500, LabelEN: "Max playlist tracks", LabelRU: "Макс. треков в плейлисте"},
	// pinterest
	{RedisKey: "pinterest.enabled", ConfigField: "PinterestEnabled", Service: "pinterest", ValueType: TypeBool, LabelEN: "Enabled", LabelRU: "Включено"},
	{RedisKey: "pinterest.timeout_sec", ConfigField: "PinterestTimeoutSeconds", Service: "pinterest", ValueType: TypeInt, MinValue: 5, MaxValue: 300, LabelEN: "Timeout", LabelRU: "Таймаут", Unit: "sec"},
}

var byKey map[string]Setting

func init() {
	byKey = make(map[string]Setting, len(settings))
	for _, s := range settings {
		byKey[s.RedisKey] = s
	}
}

func Lookup(key string) (Setting, bool) {
	s, ok := byKey[key]
	return s, ok
}

func ServiceSettings(service string) []Setting {
	var out []Setting
	for _, s := range settings {
		if s.Service == service {
			out = append(out, s)
		}
	}
	return out
}

func ServiceLabel(service, lang string) string {
	labels := map[string][2]string{
		"youtube":    {"YouTube", "YouTube"},
		"tiktok":     {"TikTok", "TikTok"},
		"instagram":  {"Instagram", "Instagram"},
		"x":          {"X / Twitter", "X / Twitter"},
		"spotify":    {"Spotify", "Spotify"},
		"soundcloud": {"SoundCloud", "SoundCloud"},
		"pinterest":  {"Pinterest", "Pinterest"},
		"global":     {"Global", "Глобально"},
	}
	if pair, ok := labels[service]; ok {
		if lang == "ru" {
			return pair[1]
		}
		return pair[0]
	}
	return service
}

func KindLabel(s Setting, lang string) string {
	if lang == "ru" {
		return s.LabelRU
	}
	return s.LabelEN
}
