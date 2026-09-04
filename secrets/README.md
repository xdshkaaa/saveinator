# secrets/

Runtime secrets mounted read-only into containers (`./secrets:/secrets:ro`).
Everything here except this README is gitignored — never commit cookies or tokens.

| File                    | Used by                                    |
|-------------------------|--------------------------------------------|
| `tiktok_cookies.txt`    | TikTok downloads via yt-dlp (`TIKTOK_COOKIES_PATH`) |
| `instagram_cookies.txt` | Instagram downloads via yt-dlp (`INSTAGRAM_COOKIES_PATH`) |
| `reddit_cookies.txt`    | Reddit downloads via yt-dlp (`REDDIT_COOKIES_PATH`) — required since Reddit started demanding auth |

Cookies must be in Netscape cookie-file format (export from a logged-in browser session).
