from aiogram.types import InlineKeyboardButton, InlineKeyboardMarkup

QUALITY_OPTIONS: tuple[tuple[str, str], ...] = (
    ("1080p", "quality:1080"),
    ("720p", "quality:720"),
    ("480p", "quality:480"),
)

RATIO_OPTIONS: tuple[tuple[str, str], ...] = (
    ("16:9", "ratio:16_9"),
    ("21:9", "ratio:21_9"),
    ("9:16", "ratio:9_16"),
)


def get_quality_keyboard() -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(
        inline_keyboard=[
            [InlineKeyboardButton(text=label, callback_data=data) for label, data in QUALITY_OPTIONS]
        ]
    )


def get_ratio_keyboard() -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(
        inline_keyboard=[
            [InlineKeyboardButton(text=label, callback_data=data) for label, data in RATIO_OPTIONS]
        ]
    )


def format_ratio_label(aspect_ratio: str) -> str:
    return aspect_ratio.replace("_", ":")
