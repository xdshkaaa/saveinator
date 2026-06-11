from aiogram import Router, F
from aiogram.types import Message, CallbackQuery, InlineKeyboardMarkup, InlineKeyboardButton
from aiogram.filters import Command
from aiogram.fsm.context import FSMContext
from aiogram.fsm.state import State, StatesGroup

from db.models import User, Language
from db.session import async_session_factory
from bot.locale import get
from bot.metrics import record_command

onboarding_router = Router()


class Onboarding(StatesGroup):
    choosing_language = State()


def language_keyboard() -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(inline_keyboard=[
        [
            InlineKeyboardButton(
                text=get("onboarding.btn_en", "en"),
                callback_data="lang|en",
            ),
            InlineKeyboardButton(
                text=get("onboarding.btn_ru", "en"),
                callback_data="lang|ru",
            ),
        ]
    ])


@onboarding_router.message(Command("start"))
async def cmd_start(message: Message, state: FSMContext, lang: str = "en"):
    record_command("start")
    user = message.from_user
    if not user:
        return

    async with async_session_factory() as session:
        existing = await session.get(User, user.id)
        if existing:
            await message.answer(get("onboarding.welcome", existing.language.value))
            return

    await state.set_state(Onboarding.choosing_language)
    await message.answer(
        get("onboarding.language_prompt", "en"),
        reply_markup=language_keyboard(),
    )


@onboarding_router.callback_query(F.data.startswith("lang|"), Onboarding.choosing_language)
async def language_chosen(callback: CallbackQuery, state: FSMContext):
    lang_code = callback.data.split("|")[1]
    if lang_code not in ("en", "ru"):
        await callback.answer()
        return

    user = callback.from_user
    async with async_session_factory() as session:
        user_obj = User(
            id=user.id,
            username=user.username,
            first_name=user.first_name,
            language=Language(lang_code),
        )
        session.add(user_obj)
        await session.commit()

    await state.clear()
    await callback.message.edit_text(
        get("onboarding.welcome", lang_code),
        reply_markup=None,
    )
    await callback.answer()
