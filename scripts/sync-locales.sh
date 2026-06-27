#!/usr/bin/env bash
# Copy locale keys from Python locales/ into Go embed locales/ (missing keys only).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required"
  exit 1
fi

merge_lang() {
  local lang="$1"
  local src="$ROOT/locales/${lang}.json"
  local dst="$ROOT/go/internal/locale/locales/${lang}.json"

  if [[ ! -f "$src" || ! -f "$dst" ]]; then
    echo "Missing $src or $dst"
    exit 1
  fi

  local merged
  merged="$(jq -s '.[0] * .[1]' "$dst" "$src")"
  if [[ "$merged" == "$(<"$dst")" ]]; then
    echo "$lang: already in sync"
  else
    echo "$merged" >"$dst"
    echo "$lang: merged missing keys from Python → Go"
  fi
}

merge_lang en
merge_lang ru

echo ""
echo "Run scripts/check-parity.sh to verify"
