import pytest
from bot.locale import get


class TestLocale:
    def test_en_simple_key(self):
        text = get("errors.unsupported", "en")
        assert "Unsupported" in text

    def test_en_nested_key(self):
        text = get("onboarding.btn_en", "en")
        assert "English" in text

    def test_ru_simple_key(self):
        text = get("errors.unsupported", "ru")
        assert "Неподдерживаемая" in text

    def test_format_args(self):
        text = get("errors.rate_limit", "en", count=5, window=60)
        assert "5" in text
        assert "60" in text

    def test_fallback_en_for_missing(self):
        text = get("errors.rate_limit", lang="zz", count=3, window=30)
        assert "3" in text

    def test_caching(self):
        t1 = get("onboarding.welcome", "en")
        t2 = get("onboarding.welcome", "en")
        assert t1 == t2
