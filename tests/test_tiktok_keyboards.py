from bot.services.tiktok_keyboards import (
    build_carousel_images_callback_data,
    build_carousel_images_keyboard,
    parse_carousel_images_callback,
)


def test_build_carousel_images_callback_data():
    data = build_carousel_images_callback_data(123456, "abc123token")
    assert data == "ttk:img:123456:abc123token"
    assert len(data.encode()) <= 64


def test_parse_carousel_images_callback():
    parsed = parse_carousel_images_callback("ttk:img:42:tok")
    assert parsed is not None
    assert parsed.user_id == 42
    assert parsed.token == "tok"


def test_parse_carousel_images_callback_rejects_invalid():
    assert parse_carousel_images_callback(None) is None
    assert parse_carousel_images_callback("quality:720") is None
    assert parse_carousel_images_callback("ttk:img:bad:tok") is None


def test_build_carousel_images_keyboard_uses_locale(monkeypatch):
    monkeypatch.setattr(
        "bot.services.tiktok_keyboards.get",
        lambda key, lang: "Download carousel photos" if key == "tiktok.btn_carousel_images" else key,
    )
    keyboard = build_carousel_images_keyboard("en", 1, "tok123")
    button = keyboard.inline_keyboard[0][0]
    assert button.text == "Download carousel photos"
    assert button.callback_data == "ttk:img:1:tok123"
