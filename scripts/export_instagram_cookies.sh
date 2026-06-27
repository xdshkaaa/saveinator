#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/secrets/instagram_cookies.txt}"
BROWSER="${INSTAGRAM_COOKIES_FROM_BROWSER:-chrome}"
PROBE_URL="${INSTAGRAM_COOKIES_PROBE_URL:-https://www.instagram.com/reel/DaAl-AKqLRF/}"

mkdir -p "$(dirname "$OUT")"

if [[ -x "$ROOT/.venv/bin/yt-dlp" ]]; then
  YTDLP="$ROOT/.venv/bin/yt-dlp"
else
  YTDLP="yt-dlp"
fi

# yt-dlp writes the cookie jar during --cookies-from-browser. Do not pass an
# existing empty file via mktemp — yt-dlp treats --cookies as input and fails
# with "does not look like a Netscape format cookies file".
rm -f "$OUT"

set +e
EXPORT_LOG=$("$YTDLP" --no-update \
  --cookies-from-browser "$BROWSER" \
  --cookies "$OUT" \
  --skip-download \
  --ignore-no-formats-error \
  "$PROBE_URL" 2>&1)
EXPORT_STATUS=$?
set -e

if [[ $EXPORT_STATUS -ne 0 ]] || [[ ! -s "$OUT" ]] || ! grep -q 'sessionid' "$OUT"; then
  echo "yt-dlp cookie export failed:" >&2
  echo "$EXPORT_LOG" >&2
  echo >&2
  echo "Ensure you are logged into Instagram in $BROWSER." >&2
  echo "On macOS, grant Full Disk Access to Terminal/Cursor:" >&2
  echo "  System Settings → Privacy & Security → Full Disk Access" >&2
  exit 1
fi

echo "Exported Instagram cookies to $OUT"
