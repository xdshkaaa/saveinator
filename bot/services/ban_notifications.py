import structlog
from aiogram import Bot
from aiogram.types import Message

from bot.config import settings
from bot.locale import get

logger = structlog.get_logger()


def _user_label(message: Message) -> str:
    user = message.from_user
    if user is None:
        return "unknown"
    parts = [str(user.id)]
    if user.username:
        parts.append(f"@{user.username}")
    if user.full_name:
        parts.append(user.full_name)
    return " · ".join(parts)


def _chat_label(message: Message) -> str:
    chat = message.chat
    chat_type = chat.type if chat else "unknown"
    chat_id = chat.id if chat else "?"
    title = chat.title if chat and chat.title else ""
    if title:
        return f"{chat_type} ({chat_id}, {title})"
    return f"{chat_type} ({chat_id})"


async def notify_admin_banned_message(bot: Bot, message: Message, lang: str = "ru") -> None:
    header = get(
        "ban.admin_notice",
        lang,
        user=_user_label(message),
        chat=_chat_label(message),
    )
    try:
        await bot.send_message(settings.admin_telegram_id, header)
        await bot.forward_message(
            chat_id=settings.admin_telegram_id,
            from_chat_id=message.chat.id,
            message_id=message.message_id,
        )
    except Exception:
        logger.warning(
            "failed to notify admin about banned user message",
            user_id=message.from_user.id if message.from_user else None,
            chat_id=message.chat.id,
            exc_info=True,
        )
