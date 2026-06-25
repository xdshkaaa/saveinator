from dataclasses import dataclass

from aiogram.types import InlineKeyboardButton, InlineKeyboardMarkup

from bot.locale import get

_CALLBACK_PREFIX = "ttk:img"


@dataclass(frozen=True)
class TikTokCarouselImagesCallback:
    user_id: int
    token: str


def build_carousel_images_callback_data(user_id: int, token: str) -> str:
    return f"{_CALLBACK_PREFIX}:{user_id}:{token}"


def parse_carousel_images_callback(data: str | None) -> TikTokCarouselImagesCallback | None:
    if not data:
        return None
    parts = data.split(":", 3)
    if len(parts) != 4 or parts[0] != "ttk" or parts[1] != "img":
        return None
    _, _, user_id_value, token = parts
    if not token:
        return None
    try:
        user_id = int(user_id_value)
    except ValueError:
        return None
    return TikTokCarouselImagesCallback(user_id=user_id, token=token)


def build_carousel_images_keyboard(
    lang: str,
    user_id: int,
    token: str,
) -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(
        inline_keyboard=[
            [
                InlineKeyboardButton(
                    text=get("tiktok.btn_carousel_images", lang),
                    callback_data=build_carousel_images_callback_data(user_id, token),
                )
            ]
        ]
    )
