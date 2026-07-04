#!/usr/bin/env bash
# Verify all locale files in go/internal/locale/locales/ share the same keys.
# en.json is the reference; every other *.json must match it exactly.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIR="$ROOT/go/internal/locale/locales"
REF="$DIR/en.json"

if [[ ! -f "$REF" ]]; then
  echo "MISSING: $REF"
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required"
  exit 1
fi

flatten_keys() {
  jq -r 'paths(scalars) | map(tostring) | join(".")' "$1" | sort -u
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

flatten_keys "$REF" >"$tmp/en"

errors=0
for f in "$DIR"/*.json; do
  code="$(basename "$f" .json)"
  [[ "$code" == "en" ]] && continue
  flatten_keys "$f" >"$tmp/$code"
  only_en="$(comm -23 "$tmp/en" "$tmp/$code" || true)"
  only_other="$(comm -13 "$tmp/en" "$tmp/$code" || true)"
  if [[ -n "$only_en" || -n "$only_other" ]]; then
    echo "=== en vs $code ==="
    if [[ -n "$only_en" ]]; then
      echo "Missing in $code:"
      echo "$only_en" | sed 's/^/  /'
    fi
    if [[ -n "$only_other" ]]; then
      echo "Only in $code:"
      echo "$only_other" | sed 's/^/  /'
    fi
    errors=1
  fi
done

if [[ "$errors" -eq 0 ]]; then
  echo "OK: all locale keys in sync"
  exit 0
fi

echo ""
echo "Fix: add the missing keys to the listed locale files"
exit 1
