#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/secrets/tiktok_cookies.txt}"
BROWSER="${TIKTOK_COOKIES_FROM_BROWSER:-chrome}"
PROBE_URL="${TIKTOK_COOKIES_PROBE_URL:-https://www.tiktok.com/@tiktok/video/7106594319421886977}"

mkdir -p "$(dirname "$OUT")"

if [[ -x "$ROOT/.venv/bin/yt-dlp" ]]; then
  YTDLP="$ROOT/.venv/bin/yt-dlp"
else
  YTDLP="yt-dlp"
fi

# yt-dlp writes the cookie jar during --cookies-from-browser. The probe URL
# only triggers extraction; homepage URLs fail as "Unsupported URL".
set +e
"$YTDLP" --no-update \
  --cookies-from-browser "$BROWSER" \
  --cookies "$OUT" \
  --skip-download \
  --ignore-no-formats-error \
  "$PROBE_URL" >/dev/null 2>&1
set -e

if [[ ! -s "$OUT" ]]; then
  echo "Cookie export failed: $OUT was not created" >&2
  exit 1
fi

echo "Exported TikTok cookies to $OUT"
