package runtime

type ValueType string

const (
	TypeInt  ValueType = "int"
	TypeBool ValueType = "bool"
	TypeEnum ValueType = "enum"
	TypeList ValueType = "list"
)

type Setting struct {
	RedisKey     string
	ConfigField  string
	Service      string
	ValueType    ValueType
	MinValue     int
	MaxValue     int
	DefaultInt   int
	DefaultBool  bool
	DefaultStr   string
	Allowed      []string
	LabelEN      string
	LabelRU      string
	Unit         string
}

var ServiceOrder = []string{
	"youtube", "tiktok", "x", "spotify", "soundcloud", "pinterest", "instagram",
}

func intSetting(key, field, service, en, ru, unit string, min, max, def int) Setting {
	return Setting{RedisKey: key, ConfigField: field, Service: service, ValueType: TypeInt, MinValue: min, MaxValue: max, DefaultInt: def, LabelEN: en, LabelRU: ru, Unit: unit}
}

func boolSetting(key, field, service, en, ru string, def bool) Setting {
	return Setting{RedisKey: key, ConfigField: field, Service: service, ValueType: TypeBool, DefaultBool: def, LabelEN: en, LabelRU: ru}
}

var settings = []Setting{
	// global
	intSetting("global.document_limit_mb", "SendDocumentLimitMB", "global", "Document limit", "Лимит документов", "MB", 1, 1999, 1999),
	intSetting("global.telegram_upload_limit_mb", "TelegramUploadLimitMB", "global", "Telegram upload limit", "Лимит загрузки Telegram", "MB", 1, 1999, 50),
	intSetting("global.default_timeout_sec", "DownloadTimeoutSeconds", "global", "Default timeout", "Таймаут по умолчанию", "sec", 10, 3600, 60),
	intSetting("global.broadcast_delay_ms", "BroadcastDelayMS", "global", "Broadcast delay", "Задержка рассылки", "ms", 20, 5000, 50),
	intSetting("global.broadcast_batch_size", "BroadcastBatchSize", "global", "Broadcast batch size", "Размер пачки рассылки", "", 1, 100, 20),

	// youtube
	intSetting("youtube.max_file_mb", "YouTubeMaxFileSizeMB", "youtube", "Max file", "Лимит файла", "MB", 1, 1999, 1999),
	intSetting("youtube.timeout_sec", "YouTubeDownloadTimeoutSeconds", "youtube", "Timeout", "Таймаут", "sec", 30, 3600, 600),
	boolSetting("youtube.transcode_enabled", "YouTubeTranscodeEnabled", "youtube", "Transcode enabled", "Транскодинг", true),
	intSetting("youtube.max_duration_sec", "YouTubeMaxDurationSec", "youtube", "Max duration", "Макс. длительность", "sec", 0, 86400, 0),
	boolSetting("youtube.compress_long_enabled", "YouTubeCompressLongEnabled", "youtube", "Compress long videos", "Сжатие длинных видео", true),
	intSetting("youtube.compress_min_duration_sec", "YouTubeCompressMinDurationSec", "youtube", "Compress min duration", "Мин. длительность сжатия", "sec", 60, 86400, 600),
	boolSetting("youtube.mp3_enabled", "", "youtube", "Mp3 button", "Кнопка Mp3", true),
	boolSetting("youtube.trim_enabled", "", "youtube", "Trim button", "Кнопка обрезки", true),
	{RedisKey: "youtube.allowed_qualities", Service: "youtube", ValueType: TypeList, DefaultStr: "144,240,360,480,720,1080", Allowed: []string{"144", "240", "360", "480", "720", "1080"}, LabelEN: "Allowed qualities", LabelRU: "Доступные качества"},
	{RedisKey: "youtube.allowed_ratios", Service: "youtube", ValueType: TypeList, DefaultStr: "16_9,21_9,9_16", Allowed: []string{"16_9", "21_9", "9_16"}, LabelEN: "Allowed ratios", LabelRU: "Доступные соотношения"},

	// tiktok
	intSetting("tiktok.max_file_mb", "SendVideoLimitMB", "tiktok", "Max file", "Лимит файла", "MB", 1, 500, 50),
	intSetting("tiktok.timeout_sec", "DownloadTimeoutSeconds", "tiktok", "Timeout", "Таймаут", "sec", 10, 300, 60),
	intSetting("tiktok.max_duration_sec", "TikTokMaxDurationSec", "tiktok", "Max duration", "Макс. длительность", "sec", 0, 600, 0),
	boolSetting("tiktok.allow_photo_slideshows", "TikTokAllowPhotoSlideshows", "tiktok", "Allow photo slideshows", "Фото-слайдшоу", true),
	boolSetting("tiktok.fallback_to_document", "TikTokFallbackToDocument", "tiktok", "Fallback to document", "Документ как fallback", true),
	intSetting("tiktok.carousel_max_items", "TikTokCarouselMaxItems", "tiktok", "Max carousel images", "Макс. фото в карусели", "", 1, 50, 20),
	boolSetting("tiktok.carousel_audio_enabled", "TikTokCarouselAudioEnabled", "tiktok", "Carousel audio", "Аудио карусели", true),

	// x
	intSetting("x.max_file_mb", "SendVideoLimitMB", "x", "Max file", "Лимит файла", "MB", 1, 500, 50),
	intSetting("x.timeout_sec", "DownloadTimeoutSeconds", "x", "Timeout", "Таймаут", "sec", 10, 300, 60),
	intSetting("x.max_items_per_post", "XMaxItemsPerPost", "x", "Max items per post", "Элементов на пост", "", 1, 10, 4),
	boolSetting("x.allow_gif", "XAllowGIF", "x", "Allow GIF", "GIF", true),
	boolSetting("x.allow_video", "XAllowVideo", "x", "Allow video", "Видео", true),
	boolSetting("x.fallback_to_document", "XFallbackToDocument", "x", "Fallback to document", "Документ как fallback", true),
	boolSetting("x.auto_translate", "XAutoTranslate", "x", "Auto-translate post text", "Автоперевод текста поста", true),

	// spotify
	boolSetting("spotify.enabled", "SpotifyEnabled", "spotify", "Enabled", "Включено", false),
	boolSetting("spotify.download_enabled", "SpotifyDownloadEnabled", "spotify", "Download enabled", "Скачивание", true),
	intSetting("spotify.max_file_mb", "SendDocumentLimitMB", "spotify", "Max file", "Лимит файла", "MB", 1, 1999, 1999),
	intSetting("spotify.track_timeout_sec", "SpotifyTrackTimeoutSeconds", "spotify", "Track timeout", "Таймаут трека", "sec", 10, 300, 60),
	intSetting("spotify.api_timeout_sec", "SpotifyAPITimeoutSeconds", "spotify", "API timeout", "Таймаут API", "sec", 5, 60, 15),
	intSetting("spotify.max_tracks_per_album", "SpotifyLockMaxTracks", "spotify", "Max tracks per album", "Треков на альбом", "", 1, 100, 50),
	intSetting("spotify.download_concurrency", "SpotifyDownloadConcurrency", "spotify", "Download concurrency", "Одновременных загрузок", "", 1, 5, 2),

	// soundcloud
	boolSetting("soundcloud.enabled", "SoundCloudEnabled", "soundcloud", "Enabled", "Включено", true),
	boolSetting("soundcloud.download_enabled", "SoundCloudDownloadEnabled", "soundcloud", "Download enabled", "Скачивание", false),
	intSetting("soundcloud.max_file_mb", "SendVideoLimitMB", "soundcloud", "Max file", "Лимит файла", "MB", 1, 500, 50),
	intSetting("soundcloud.track_timeout_sec", "SoundCloudTrackTimeoutSeconds", "soundcloud", "Track timeout", "Таймаут трека", "sec", 10, 300, 30),
	intSetting("soundcloud.max_tracks_per_playlist", "SoundCloudMaxTracks", "soundcloud", "Max playlist tracks", "Макс. треков в плейлисте", "", 1, 500, 100),
	{RedisKey: "soundcloud.audio_format", ConfigField: "SoundCloudDLOutputFormat", Service: "soundcloud", ValueType: TypeEnum, DefaultStr: "mp3", Allowed: []string{"mp3", "opus", "aac", "flac", "wav"}, LabelEN: "Audio format", LabelRU: "Формат аудио"},
	intSetting("soundcloud.download_concurrency", "SoundCloudDownloadConcurrency", "soundcloud", "Download concurrency", "Одновременных загрузок", "", 1, 5, 1),

	// pinterest
	boolSetting("pinterest.enabled", "PinterestEnabled", "pinterest", "Enabled", "Включено", true),
	intSetting("pinterest.timeout_sec", "PinterestTimeoutSeconds", "pinterest", "Timeout", "Таймаут", "sec", 5, 300, 30),
	intSetting("pinterest.max_file_mb", "SendVideoLimitMB", "pinterest", "Max file", "Лимит файла", "MB", 1, 500, 50),
	intSetting("pinterest.max_items_per_board", "PinterestMaxItems", "pinterest", "Max items per board", "Элементов на доску", "", 1, 50, 10),
	boolSetting("pinterest.download_images", "PinterestDownloadImages", "pinterest", "Download images", "Скачивать изображения", true),
	boolSetting("pinterest.download_videos", "PinterestDownloadVideos", "pinterest", "Download videos", "Скачивать видео", true),

	// instagram
	boolSetting("instagram.enabled", "InstagramEnabled", "instagram", "Enabled", "Включено", true),
	intSetting("instagram.timeout_sec", "InstagramTimeoutSeconds", "instagram", "Timeout", "Таймаут", "sec", 10, 600, 60),
	intSetting("instagram.max_file_mb", "InstagramMaxFileMB", "instagram", "Max file", "Лимит файла", "MB", 1, 500, 50),
	intSetting("instagram.carousel_max_items", "InstagramCarouselMaxItems", "instagram", "Max carousel images", "Макс. фото в карусели", "", 1, 50, 20),
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
		"x":          {"X / Twitter", "X / Twitter"},
		"spotify":    {"Spotify", "Spotify"},
		"soundcloud": {"SoundCloud", "SoundCloud"},
		"pinterest":  {"Pinterest", "Pinterest"},
		"instagram":  {"Instagram", "Instagram"},
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
