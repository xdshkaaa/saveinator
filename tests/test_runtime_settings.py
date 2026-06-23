"""Tests for the expanded runtime settings system."""

import pytest

from bot.services.runtime_settings import (
    RUNTIME_SETTINGS,
    validate_value,
    format_value,
    setting_definition,
    service_settings,
    _serialise,
    _deserialise,
    RuntimeSettingDef,
)
from bot.handlers.admin import _parse_typed_value


# ---------------------------------------------------------------------------
# Registry integrity
# ---------------------------------------------------------------------------


def test_all_settings_have_labels():
    """Every setting must have both EN and RU labels."""
    for defn in RUNTIME_SETTINGS:
        assert defn.label_en, f"{defn.redis_key} missing label_en"
        assert defn.label_ru, f"{defn.redis_key} missing label_ru"


def test_all_settings_have_valid_type():
    """Every setting must have a valid value_type."""
    valid_types = {"int", "bool", "enum", "list"}
    for defn in RUNTIME_SETTINGS:
        assert defn.value_type in valid_types, f"{defn.redis_key} has invalid type {defn.value_type}"


def test_global_settings_present():
    """Global settings should include broadcast settings."""
    global_svc = service_settings("global")
    keys = {s.redis_key for s in global_svc}
    assert "global.broadcast_delay_ms" in keys
    assert "global.broadcast_batch_size" in keys
    assert "global.default_timeout_sec" in keys
    assert "global.document_limit_mb" in keys
    assert "global.telegram_upload_limit_mb" in keys


def test_youtube_has_all_settings():
    """YouTube should have quality, ratio, transcode, etc."""
    svc = service_settings("youtube")
    keys = {s.redis_key for s in svc}
    assert "youtube.allowed_qualities" in keys
    assert "youtube.default_quality" in keys
    assert "youtube.allowed_ratios" in keys
    assert "youtube.transcode_enabled" in keys
    assert "youtube.max_duration_sec" in keys


def test_lists_have_allowed_values():
    """List-type settings must have allowed_values specified."""
    for defn in RUNTIME_SETTINGS:
        if defn.value_type == "list":
            assert defn.allowed_values, f"{defn.redis_key} is list but no allowed_values"


def test_enums_have_allowed_values():
    """Enum-type settings must have allowed_values specified."""
    for defn in RUNTIME_SETTINGS:
        if defn.value_type == "enum":
            assert defn.allowed_values, f"{defn.redis_key} is enum but no allowed_values"


# ---------------------------------------------------------------------------
# Validation
# ---------------------------------------------------------------------------


class TestValidateValue:
    def test_valid_int(self):
        defn = setting_definition("youtube.max_file_mb")
        assert defn is not None
        assert validate_value(defn, "500") is None
        assert validate_value(defn, "1") is None
        assert validate_value(defn, "1999") is None

    def test_int_out_of_range(self):
        defn = setting_definition("youtube.max_file_mb")
        assert defn is not None
        assert validate_value(defn, "0") is not None
        assert validate_value(defn, "2000") is not None
        assert validate_value(defn, "-1") is not None

    def test_invalid_int(self):
        defn = setting_definition("youtube.max_file_mb")
        assert defn is not None
        assert validate_value(defn, "abc") is not None
        assert validate_value(defn, "") is not None

    def test_valid_bool(self):
        defn = setting_definition("youtube.transcode_enabled")
        assert defn is not None
        assert validate_value(defn, "1") is None
        assert validate_value(defn, "0") is None
        assert validate_value(defn, "true") is None
        assert validate_value(defn, "false") is None
        assert validate_value(defn, "yes") is None
        assert validate_value(defn, "no") is None

    def test_invalid_bool(self):
        defn = setting_definition("youtube.transcode_enabled")
        assert defn is not None
        assert validate_value(defn, "maybe") is not None
        assert validate_value(defn, "2") is not None

    def test_valid_enum(self):
        defn = setting_definition("youtube.default_quality")
        assert defn is not None
        assert validate_value(defn, "1080") is None
        assert validate_value(defn, "720") is None
        assert validate_value(defn, "ask") is None

    def test_invalid_enum(self):
        defn = setting_definition("youtube.default_quality")
        assert defn is not None
        assert validate_value(defn, "2160") is not None
        assert validate_value(defn, "auto") is not None

    def test_valid_list(self):
        defn = setting_definition("youtube.allowed_qualities")
        assert defn is not None
        assert validate_value(defn, "1080,720") is None
        assert validate_value(defn, "720") is None
        # All values in allowed list
        assert validate_value(defn, "1080,720,480") is None

    def test_invalid_list(self):
        defn = setting_definition("youtube.allowed_qualities")
        assert defn is not None
        assert validate_value(defn, "2160") is not None
        assert validate_value(defn, "1080,2160") is not None


# ---------------------------------------------------------------------------
# Serialisation
# ---------------------------------------------------------------------------


class TestSerialise:
    def test_int(self):
        assert _serialise(42) == "42"

    def test_bool(self):
        assert _serialise(True) == "1"
        assert _serialise(False) == "0"

    def test_tuple(self):
        assert _serialise(("1080", "720")) == "1080,720"

    def test_list(self):
        assert _serialise(["a", "b"]) == "a,b"

    def test_string(self):
        assert _serialise("1080") == "1080"

    def test_empty_tuple(self):
        assert _serialise(()) == ""


class TestDeserialise:
    def test_none_returns_none(self):
        defn = setting_definition("youtube.max_file_mb")
        assert _deserialise(None, defn) is None

    def test_int(self):
        defn = setting_definition("youtube.max_file_mb")
        assert _deserialise("42", defn) == 42

    def test_bool_true(self):
        defn = setting_definition("youtube.transcode_enabled")
        assert _deserialise("1", defn) is True
        assert _deserialise("true", defn) is True
        assert _deserialise("True", defn) is True

    def test_bool_false(self):
        defn = setting_definition("youtube.transcode_enabled")
        assert _deserialise("0", defn) is False
        assert _deserialise("false", defn) is False

    def test_enum(self):
        defn = setting_definition("youtube.default_quality")
        result = _deserialise("1080", defn)
        assert isinstance(result, tuple)
        assert result == ("1080",)

    def test_list(self):
        defn = setting_definition("youtube.allowed_qualities")
        result = _deserialise("1080,720", defn)
        assert result == ("1080", "720")


# ---------------------------------------------------------------------------
# Formatting
# ---------------------------------------------------------------------------


class TestFormatValue:
    def test_format_int_with_unit(self):
        defn = setting_definition("youtube.max_file_mb")
        assert defn is not None
        assert format_value(500, defn) == "500 MB"
        assert format_value(500, defn, "ru") == "500 MB"

    def test_format_bool_en(self):
        defn = setting_definition("youtube.transcode_enabled")
        assert defn is not None
        assert "On" in format_value(True, defn, "en")
        assert "Off" in format_value(False, defn, "en")

    def test_format_bool_ru(self):
        defn = setting_definition("youtube.transcode_enabled")
        assert defn is not None
        assert "Вкл" in format_value(True, defn, "ru")
        assert "Выкл" in format_value(False, defn, "ru")

    def test_format_tuple(self):
        defn = setting_definition("youtube.allowed_qualities")
        assert defn is not None
        assert format_value(("1080", "720"), defn) == "1080, 720"

    def test_format_none(self):
        defn = setting_definition("youtube.max_file_mb")
        assert defn is not None
        assert format_value(None, defn) == "—"


# ---------------------------------------------------------------------------
# Parse typed value
# ---------------------------------------------------------------------------


class TestParseTypedValue:
    def test_parse_int(self):
        defn = setting_definition("youtube.max_file_mb")
        assert defn is not None
        assert _parse_typed_value("42", defn) == 42

    def test_parse_bool_true(self):
        defn = setting_definition("youtube.transcode_enabled")
        assert defn is not None
        assert _parse_typed_value("true", defn) is True
        assert _parse_typed_value("1", defn) is True
        assert _parse_typed_value("yes", defn) is True

    def test_parse_bool_false(self):
        defn = setting_definition("youtube.transcode_enabled")
        assert defn is not None
        assert _parse_typed_value("false", defn) is False
        assert _parse_typed_value("0", defn) is False

    def test_parse_enum(self):
        defn = setting_definition("youtube.default_quality")
        assert defn is not None
        assert _parse_typed_value("1080", defn) == ("1080",)

    def test_parse_list(self):
        defn = setting_definition("youtube.allowed_qualities")
        assert defn is not None
        assert _parse_typed_value("1080,720,480", defn) == ("1080", "720", "480")


# ---------------------------------------------------------------------------
# Meta: all settings are reachable
# ---------------------------------------------------------------------------


def test_all_settings_are_indexed():
    """Every setting in RUNTIME_SETTINGS must be findable by redis_key."""
    for defn in RUNTIME_SETTINGS:
        found = setting_definition(defn.redis_key)
        assert found is not None
        assert found == defn


def test_all_settings_have_min_max_for_int():
    """Int-type settings should have min/max values."""
    for defn in RUNTIME_SETTINGS:
        if defn.value_type == "int":
            assert defn.min_value is not None, f"{defn.redis_key} missing min_value"
            assert defn.max_value is not None, f"{defn.redis_key} missing max_value"


def test_no_sensitive_settings_exposed():
    """Sensitive=true settings should not appear in the registry for admin."""
    sensitive = [s for s in RUNTIME_SETTINGS if s.sensitive]
    assert len(sensitive) == 0, "Sensitive settings should not exist in runtime registry"
