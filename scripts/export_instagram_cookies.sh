#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/secrets/instagram_cookies.txt}"
BROWSER="${INSTAGRAM_COOKIES_FROM_BROWSER:-chrome}"
PROBE_URL="${INSTAGRAM_COOKIES_PROBE_URL:-https://www.instagram.com/reel/DaAl-AKqLRF/}"

mkdir -p "$(dirname "$OUT")"

YTDLP=(yt-dlp)
if [[ -x "$ROOT/.venv/bin/python" ]] && "$ROOT/.venv/bin/python" -c "import yt_dlp" >/dev/null 2>&1; then
  YTDLP=("$ROOT/.venv/bin/python" -m yt_dlp)
fi

TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

set +e
EXPORT_LOG=$("${YTDLP[@]}" --no-update \
  --cookies-from-browser "$BROWSER" \
  --cookies "$TMP" \
  --skip-download \
  --ignore-no-formats-error \
  "$PROBE_URL" 2>&1)
EXPORT_STATUS=$?
set -e

if [[ $EXPORT_STATUS -ne 0 ]] || [[ ! -s "$TMP" ]] || ! grep -q 'sessionid' "$TMP"; then
  PYTHON="$ROOT/.venv/bin/python"
  if [[ -x "$PYTHON" ]]; then
    set +e
    "$PYTHON" -m pip install -q browser-cookie3 2>/dev/null
    BROWSER_COOKIE3_LOG=$("$PYTHON" - "$TMP" "$BROWSER" <<'PY' 2>&1
import sys
from http.cookiejar import MozillaCookieJar

out, browser = sys.argv[1], sys.argv[2]
import browser_cookie3

loaders = {
    "chrome": browser_cookie3.chrome,
    "chromium": browser_cookie3.chromium,
    "brave": browser_cookie3.brave,
    "edge": browser_cookie3.edge,
    "firefox": browser_cookie3.firefox,
    "safari": browser_cookie3.safari,
}
loader = loaders.get(browser, browser_cookie3.chrome)
cj = loader(domain_name="instagram.com")
names = {c.name for c in cj}
if "sessionid" not in names:
    raise SystemExit("sessionid cookie missing")

jar = MozillaCookieJar(out)
for cookie in cj:
    if "instagram.com" in cookie.domain:
        jar.set_cookie(cookie)
jar.save(ignore_discard=True, ignore_expires=True)
PY
)
    BROWSER_COOKIE3_STATUS=$?
    set -e
    if [[ $BROWSER_COOKIE3_STATUS -eq 0 ]] && [[ -s "$TMP" ]] && grep -q 'sessionid' "$TMP"; then
      mv "$TMP" "$OUT"
      trap - EXIT
      echo "Exported Instagram cookies to $OUT (via browser-cookie3)"
      exit 0
    fi
    if [[ -n "${BROWSER_COOKIE3_LOG:-}" ]]; then
      echo "$BROWSER_COOKIE3_LOG" >&2
    fi
  fi

  if [[ $EXPORT_STATUS -ne 0 ]]; then
    echo "yt-dlp cookie export failed:" >&2
    echo "$EXPORT_LOG" >&2
    echo >&2
  fi
  echo "Ensure you are logged into Instagram in $BROWSER." >&2
  echo "On macOS, grant Full Disk Access to Terminal/Cursor:" >&2
  echo "  System Settings → Privacy & Security → Full Disk Access" >&2
  exit 1
fi

mv "$TMP" "$OUT"
trap - EXIT

echo "Exported Instagram cookies to $OUT"
