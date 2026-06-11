from unittest.mock import MagicMock, patch

import pytest

from bot.services.pinterest_pin_fetcher import extract_pin_id, fetch_pin_media


class TestExtractPinId:
    def test_standard_pin_url(self):
        assert extract_pin_id("https://ru.pinterest.com/pin/607845280985287827/") == "607845280985287827"

    def test_missing_pin(self):
        assert extract_pin_id("https://pinterest.com/user/board/") is None


class TestFetchPinMedia:
    def test_returns_single_main_pin_media(self):
        pin_payload = {
            "resource_response": {
                "data": {
                    "id": "607845280985287827",
                    "grid_title": "date w Kuriyama Mirai",
                    "description": "Kuriyama Mirai",
                    "images": {
                        "orig": {
                            "url": "https://i.pinimg.com/originals/3c/7c/93/main.png",
                            "width": 736,
                            "height": 1308,
                        }
                    },
                }
            }
        }
        mock_response = MagicMock()
        mock_response.json.return_value = pin_payload
        mock_response.raise_for_status = MagicMock()

        with patch("bot.services.pinterest_pin_fetcher.requests.Session") as session_cls:
            session = session_cls.return_value
            session.get.side_effect = [MagicMock(), mock_response]

            items = fetch_pin_media(
                "https://www.pinterest.com/pin/607845280985287827/",
                MagicMock(cookies=None),
                timeout=10,
            )

        assert len(items) == 1
        assert items[0].alt == "date w Kuriyama Mirai"
        assert "main.png" in items[0].src

    def test_raises_when_pin_id_missing(self):
        with pytest.raises(ValueError, match="Could not resolve"):
            fetch_pin_media(
                "https://www.pinterest.com/user/board/",
                MagicMock(cookies=None),
                timeout=10,
            )
