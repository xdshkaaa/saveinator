#!/usr/bin/env python3
"""Get a Yandex Music OAuth access token via the Device Flow.

Uses the public OAuth credentials of the official Android app (same ones as
community clients). Run locally, confirm the login on the printed page, copy
the token into YANDEX_MUSIC_ACCESS_TOKEN.

No third-party dependencies: stdlib urllib only.
"""

import json
import secrets
import string
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

CLIENT_ID = "23cabbbdc6cd418abb4b39c32c41195d"
CLIENT_SECRET = "53bc75238f0c4d08a118e51fe9203300"
DEVICE_NAME = "YandexMusicAPI"
OAUTH_BASE = "https://oauth.yandex.ru"
# Any known track id works: validates both the token and that the API answers
# from this network (the bot's worker uses exactly this endpoint).
VALIDATE_TRACK_URL = "https://api.music.yandex.net/tracks/154402671"


def post_form(url, data):
    body = urllib.parse.urlencode(data).encode()
    req = urllib.request.Request(url, data=body, method="POST")
    req.add_header("Content-Type", "application/x-www-form-urlencoded")
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return resp.status, json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        raw = e.read().decode(errors="replace")
        try:
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, {"error": raw}


def get_json(url, token=None):
    req = urllib.request.Request(url)
    if token:
        req.add_header("Authorization", "OAuth " + token)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return resp.status, json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        return e.code, {}


def main():
    device_id = "".join(secrets.choice(string.ascii_letters + string.digits) for _ in range(10))

    status, code = post_form(
        f"{OAUTH_BASE}/device/code",
        {"client_id": CLIENT_ID, "device_id": device_id, "device_name": DEVICE_NAME},
    )
    if status != 200 or "device_code" not in code:
        print(f"Failed to request device code (HTTP {status}): {code}", file=sys.stderr)
        sys.exit(1)

    verification = code.get("verification_url", "https://ya.ru/verify")
    sep = "&" if "?" in verification else "?"
    print()
    print("=" * 64)
    print("1. Open this link in a browser where you are logged in to Yandex:")
    print(f"     {verification}{sep}code={code['user_code']}")
    print(f"2. Or enter the code manually on {verification}:")
    print(f"     {code['user_code']}")
    print(f"   The code is valid for ~{code.get('expires_in', '?')} seconds.")
    print("Waiting for confirmation...", flush=True)

    interval = max(int(code.get("interval", 5)), 1)
    deadline = time.time() + int(code.get("expires_in", 300)) + 30

    while time.time() < deadline:
        time.sleep(interval)
        status, token = post_form(
            f"{OAUTH_BASE}/token",
            {
                "grant_type": "device_code",
                "code": code["device_code"],
                "client_id": CLIENT_ID,
                "client_secret": CLIENT_SECRET,
            },
        )
        if status == 200 and token.get("access_token"):
            print()
            print("=" * 64)
            print("access_token:")
            print(token["access_token"])
            if token.get("refresh_token"):
                print(f"refresh_token (keep private): {token['refresh_token']}")

            vstatus, _ = get_json(VALIDATE_TRACK_URL, token["access_token"])
            if vstatus == 200:
                print("API check: OK — token works from this network.")
                sys.exit(0)
            elif vstatus == 401:
                print("WARNING: API returned 401 — token may be invalid.", file=sys.stderr)
                sys.exit(2)
            else:
                print(f"NOTE: API check returned HTTP {vstatus}; "
                      "verify the token against the VPS.", file=sys.stderr)
                sys.exit(0)

        error = token.get("error", "")
        if error == "authorization_pending":
            continue
        if error == "slow_down":
            interval += 5
            continue
        print(f"\nOAuth error: {error or status}", file=sys.stderr)
        sys.exit(1)

    print("\nTimed out waiting for confirmation — run again.", file=sys.stderr)
    sys.exit(1)


if __name__ == "__main__":
    main()
