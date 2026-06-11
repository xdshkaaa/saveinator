from bot.services.pinterest_parser import is_valid_pinterest_url, parse_pinterest_url
from bot.services.pinterest_models import PinterestUrlType


class TestParsePinterestUrl:
    def test_pin(self):
        parsed = parse_pinterest_url("https://www.pinterest.com/pin/abc123/")
        assert parsed is not None
        assert parsed.url_type == PinterestUrlType.PIN

    def test_short(self):
        parsed = parse_pinterest_url("https://pin.it/shortid")
        assert parsed is not None
        assert parsed.url_type == PinterestUrlType.SHORT

    def test_board(self):
        parsed = parse_pinterest_url("https://www.pinterest.com/user/my-board/")
        assert parsed is not None
        assert parsed.url_type == PinterestUrlType.BOARD

    def test_invalid(self):
        assert parse_pinterest_url("https://example.com") is None
        assert is_valid_pinterest_url("not a url") is False

    def test_reserved_path_not_board(self):
        parsed = parse_pinterest_url("https://www.pinterest.com/search/pins/")
        assert parsed is None
