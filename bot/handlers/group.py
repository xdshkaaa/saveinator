from aiogram import Router, F
from aiogram.types import Message
from aiogram.enums import ChatType

from bot.locale import get
from bot.services.link_parser import extract_urls
from workers.tasks import download_and_send_task

group_router = Router()
group_router.message.filter(F.chat.type.in_([ChatType.PRIVATE, ChatType.GROUP, ChatType.SUPERGROUP]))


@group_router.message(F.text)
async def handle_group_message(message: Message, lang: str = "en"):
    if not message.text:
        return

    urls = extract_urls(message.text)
    if not urls:
        return

    parsed_link = urls[0]
    platform = parsed_link.platform
    url = parsed_link.url

    if platform.value == "unknown":
        await message.reply(get("errors.unsupported", lang))
        return

    status_msg = await message.reply(get("download.downloading", lang))

    download_and_send_task.delay(
        url=url,
        platform=platform.value,
        chat_id=message.chat.id,
        user_id=message.from_user.id if message.from_user else 0,
        message_id=status_msg.message_id,
        lang=lang,
    )
