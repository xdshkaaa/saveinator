import json
from pathlib import Path

LOCALES_DIR = Path(__file__).resolve().parents[1] / "locales"

REQUIRED_KEYS = (
    "card_title",
    "track_card_title",
    "duration",
    "genre",
    "tracks_count",
    "source",
    "btn_open",
    "download_starting",
    "download_track",
    "send_track",
    "download_done",
    "download_failed",
    "download_timeout",
    "no_download",
    "not_found",
    "not_configured",
    "disabled",
    "playlist_too_large",
)


class TestSoundCloudLocales:
    def test_en_and_ru_have_required_keys(self):
        for lang in ("en", "ru"):
            data = json.loads((LOCALES_DIR / f"{lang}.json").read_text(encoding="utf-8"))
            soundcloud = data["soundcloud"]
            for key in REQUIRED_KEYS:
                assert key in soundcloud, f"missing soundcloud.{key} in {lang}.json"
