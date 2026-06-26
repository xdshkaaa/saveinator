import asyncio
from collections.abc import Awaitable, Callable
from typing import Any, TypeVar

import structlog

from bot.locale import get

logger = structlog.get_logger()

TResult = TypeVar("TResult")
OnDownloadStart = Callable[[int, Any], Awaitable[None]]
DownloadFn = Callable[[int, Any, OnDownloadStart], Awaitable[TResult]]


async def edit_download_status(
    status_msg,
    text: str,
    reply_markup=None,
    *,
    clear_markup: bool = False,
) -> None:
    if reply_markup is not None or clear_markup:
        await status_msg.edit_text(text, reply_markup=reply_markup)
    else:
        await status_msg.edit_text(text)


async def run_ordered_release_download(
    *,
    tracks: list[Any],
    status_msg,
    lang: str,
    locale_prefix: str,
    cancel_keyboard,
    download_fn: DownloadFn,
    send_fn: Callable[[TResult], Awaitable[bool]],
) -> tuple[int, list[TResult | BaseException]]:
    total = len(tracks)
    completed_buffer: dict[int, TResult] = {}
    next_send_index = 1
    sent = 0
    task_results: list[TResult | BaseException] = []

    async def on_download_start(index: int, track: Any) -> None:
        await edit_download_status(
            status_msg,
            get(
                f"{locale_prefix}.download_track",
                lang,
                current=index,
                total=total,
                title=track.title,
            ),
            reply_markup=cancel_keyboard,
        )

    async def wrapped_download(index: int, track: Any) -> TResult:
        return await download_fn(index, track, on_download_start)

    tasks = [
        asyncio.create_task(wrapped_download(index, track))
        for index, track in enumerate(tracks, start=1)
    ]

    for finished in asyncio.as_completed(tasks):
        try:
            result = await finished
        except Exception as exc:
            logger.exception("unexpected track download task failure", error=exc)
            task_results.append(exc)
            continue

        task_results.append(result)
        completed_buffer[result.index] = result

        while next_send_index in completed_buffer:
            result = completed_buffer.pop(next_send_index)
            if result.error is None and result.audio_path is not None:
                await edit_download_status(
                    status_msg,
                    get(
                        f"{locale_prefix}.send_track",
                        lang,
                        current=next_send_index,
                        total=total,
                        title=result.track.title,
                    ),
                    reply_markup=cancel_keyboard,
                )
                if await send_fn(result):
                    sent += 1
            next_send_index += 1

    return sent, task_results
