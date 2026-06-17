from workers.youtube_format import build_youtube_format


def test_build_youtube_format_1080():
    fmt = build_youtube_format(1080)
    assert "height<=1080" in fmt
    assert "bestaudio" in fmt


def test_build_youtube_format_720():
    fmt = build_youtube_format(720)
    assert "height<=720" in fmt


def test_build_youtube_format_480():
    fmt = build_youtube_format(480)
    assert "height<=480" in fmt
