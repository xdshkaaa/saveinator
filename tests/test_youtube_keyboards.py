from bot.services.youtube_keyboards import (
    QUALITY_OPTIONS,
    RATIO_OPTIONS,
    format_ratio_label,
    get_quality_keyboard,
    get_ratio_keyboard,
)


def test_quality_keyboard_callback_data():
    keyboard = get_quality_keyboard()
    buttons = keyboard.inline_keyboard[0]
    assert [(button.text, button.callback_data) for button in buttons] == list(QUALITY_OPTIONS)


def test_ratio_keyboard_callback_data():
    keyboard = get_ratio_keyboard()
    buttons = keyboard.inline_keyboard[0]
    assert [(button.text, button.callback_data) for button in buttons] == list(RATIO_OPTIONS)


def test_format_ratio_label():
    assert format_ratio_label("16_9") == "16:9"
    assert format_ratio_label("9_16") == "9:16"
