import asyncio
from dataclasses import dataclass

from aiogram.types import InlineKeyboardButton, InlineKeyboardMarkup

from bot.locale import get
from bot.services.user_queue import ActiveUserDownload, UserScenario

_CALLBACK_PREFIX = "dlc"
_QUEUE_CALLBACK_PREFIX = "dlq"
_active_downloads: dict[str, asyncio.Task] = {}


@dataclass(frozen=True)
class DownloadCancelData:
    scenario: UserScenario
    user_id: int
    token: str


def build_cancel_callback_data(scenario: UserScenario, user_id: int, token: str) -> str:
    return f"{_CALLBACK_PREFIX}:{scenario.value}:{user_id}:{token}"


def build_queue_callback_data(user_id: int) -> str:
    return f"{_QUEUE_CALLBACK_PREFIX}:{user_id}"


def parse_queue_callback_data(data: str | None) -> int | None:
    if not data:
        return None
    parts = data.split(":", 1)
    if len(parts) != 2 or parts[0] != _QUEUE_CALLBACK_PREFIX:
        return None
    try:
        return int(parts[1])
    except ValueError:
        return None


def parse_cancel_callback_data(data: str | None) -> DownloadCancelData | None:
    if not data:
        return None
    parts = data.split(":", 3)
    if len(parts) != 4 or parts[0] != _CALLBACK_PREFIX:
        return None
    _, scenario_value, user_id_value, token = parts
    if not token:
        return None
    try:
        scenario = UserScenario(scenario_value)
        user_id = int(user_id_value)
    except (ValueError, TypeError):
        return None
    return DownloadCancelData(scenario=scenario, user_id=user_id, token=token)


def _task_key(scenario: UserScenario, user_id: int, token: str) -> str:
    return build_cancel_callback_data(scenario, user_id, token)


def build_cancel_keyboard(
    lang: str,
    scenario: UserScenario,
    user_id: int | None,
    token: str,
) -> InlineKeyboardMarkup | None:
    if not user_id or not token:
        return None
    return InlineKeyboardMarkup(
        inline_keyboard=[
            [
                InlineKeyboardButton(
                    text=get("download.cancel", lang),
                    callback_data=build_cancel_callback_data(scenario, user_id, token),
                )
            ]
        ]
    )


def build_download_queue_button(lang: str, user_id: int | None) -> InlineKeyboardMarkup | None:
    if not user_id:
        return None
    return InlineKeyboardMarkup(
        inline_keyboard=[
            [
                InlineKeyboardButton(
                    text=get("download.queue_button", lang),
                    callback_data=build_queue_callback_data(user_id),
                )
            ]
        ]
    )


def _scenario_label(scenario: UserScenario) -> str:
    labels = {
        UserScenario.VIDEO: "video",
        UserScenario.PINTEREST: "Pinterest",
        UserScenario.SPOTIFY: "Spotify",
        UserScenario.SOUNDCLOUD: "SoundCloud",
    }
    return labels[scenario]


def build_active_downloads_keyboard(
    lang: str,
    active: ActiveUserDownload,
) -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(
        inline_keyboard=[
            [
                InlineKeyboardButton(
                    text=get(
                        "download.queue_remove",
                        lang,
                        item=_scenario_label(active.scenario),
                    ),
                    callback_data=build_cancel_callback_data(
                        active.scenario,
                        active.user_id,
                        active.token,
                    ),
                )
            ]
        ]
    )


def register_download_task(
    scenario: UserScenario,
    user_id: int | None,
    token: str,
    task: asyncio.Task,
) -> None:
    if not user_id or not token:
        return
    _active_downloads[_task_key(scenario, user_id, token)] = task


def unregister_download_task(scenario: UserScenario, user_id: int | None, token: str) -> None:
    if not user_id or not token:
        return
    _active_downloads.pop(_task_key(scenario, user_id, token), None)


def cancel_download_task(scenario: UserScenario, user_id: int, token: str) -> bool:
    task = _active_downloads.get(_task_key(scenario, user_id, token))
    if task is None or task.done():
        return False
    return task.cancel()
