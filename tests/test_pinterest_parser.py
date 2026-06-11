from bot.services.link_parser import extract_urls
from db.models import Platform


class TestPinterestPinUrls:
    def test_standard_pin_url(self):
        links = extract_urls("https://www.pinterest.com/pin/123456789/")
        assert len(links) == 1
        assert links[0].platform == Platform.PINTEREST
        assert "pinterest.com/pin/123456789" in links[0].url

    def test_pin_url_without_trailing_slash(self):
        links = extract_urls("https://www.pinterest.com/pin/abc123-def")
        assert len(links) == 1
        assert links[0].platform == Platform.PINTEREST

    def test_pin_url_without_www(self):
        links = extract_urls("https://pinterest.com/pin/999888777/")
        assert len(links) == 1
        assert links[0].platform == Platform.PINTEREST

    def test_pin_url_with_query_params(self):
        links = extract_urls("https://www.pinterest.com/pin/123/?sender=home")
        assert len(links) == 1
        assert links[0].platform == Platform.PINTEREST

    def test_pin_it_short_url(self):
        links = extract_urls("https://pin.it/abc123XYZ")
        assert len(links) == 1
        assert links[0].platform == Platform.PINTEREST
        assert "pin.it" in links[0].url

    def test_pin_it_short_url_lowercase(self):
        links = extract_urls("check this: https://pin.it/abcdefgh")
        assert len(links) == 1
        assert links[0].platform == Platform.PINTEREST


class TestPinterestBoardUrls:
    def test_board_url(self):
        links = extract_urls("https://www.pinterest.com/username/board-name/")
        assert len(links) == 1
        assert links[0].platform == Platform.PINTEREST
        assert "username/board-name" in links[0].url

    def test_board_url_without_trailing_slash(self):
        links = extract_urls("https://www.pinterest.com/user123/my-cool-board")
        assert len(links) == 1
        assert links[0].platform == Platform.PINTEREST

    def test_board_url_without_www(self):
        links = extract_urls("https://pinterest.com/artist/landscapes/")
        assert len(links) == 1
        assert links[0].platform == Platform.PINTEREST


class TestPinterestNotMatched:
    def test_single_segment_path_not_matched(self):
        links = extract_urls("https://www.pinterest.com/username/")
        assert all(l.platform != Platform.PINTEREST for l in links)

    def test_random_site_not_matched(self):
        links = extract_urls("https://example.com/pinterest/pin/123")
        assert all(l.platform != Platform.PINTEREST for l in links)

    def test_pinterest_in_text_only(self):
        links = extract_urls("I love pinterest for ideas")
        assert len(links) == 0


class TestPinterestMixedWithOtherPlatforms:
    def test_pinterest_and_youtube(self):
        text = "https://www.pinterest.com/pin/111/ and https://youtube.com/watch?v=dQw4w9WgXcQ"
        links = extract_urls(text)
        platforms = [l.platform for l in links]
        assert Platform.PINTEREST in platforms
        assert Platform.YOUTUBE in platforms

    def test_only_first_url_returned_per_platform(self):
        text = "https://pin.it/aaa https://pin.it/bbb"
        links = extract_urls(text)
        pinterest_links = [l for l in links if l.platform == Platform.PINTEREST]
        assert len(pinterest_links) == 2
