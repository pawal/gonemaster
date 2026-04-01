#!/usr/bin/env sh
set -eu

# List missing translation keys for a locale by comparing with union of other locales.
# Usage:
#   sh tools/i18n/missing-keys.sh [--grouped] [--count-only] <locale>

grouped=0
count_only=0
target_locale=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --grouped)
      grouped=1
      ;;
    --count-only)
      count_only=1
      ;;
    -*)
      echo "error: unknown option: $1" >&2
      echo "usage: sh tools/i18n/missing-keys.sh [--grouped] [--count-only] <locale>" >&2
      exit 2
      ;;
    *)
      if [ -n "$target_locale" ]; then
        echo "error: unexpected extra argument: $1" >&2
        echo "usage: sh tools/i18n/missing-keys.sh [--grouped] [--count-only] <locale>" >&2
        exit 2
      fi
      target_locale="$1"
      ;;
  esac
  shift
done

if [ -z "$target_locale" ]; then
  echo "usage: sh tools/i18n/missing-keys.sh [--grouped] [--count-only] <locale>" >&2
  exit 2
fi

target_file="share/lang/${target_locale}.po"

if [ ! -f "$target_file" ]; then
  echo "error: locale file not found: $target_file" >&2
  exit 1
fi

tmp_base="$(mktemp)"
tmp_target="$(mktemp)"
tmp_missing="$(mktemp)"
trap 'rm -f "$tmp_base" "$tmp_target" "$tmp_missing"' EXIT

extract_keys() {
  file="$1"
  awk '
    /^#\./ {
      tag = $0
      sub(/^#\. */, "", tag)
      split(tag, parts, /[ \t]+/)
      key = toupper(parts[1])
      if (key ~ /^[A-Z0-9_]+:[A-Z0-9_]+$/) print key
      next
    }
    /^msgctxt / {
      key = $0
      sub(/^msgctxt "/, "", key)
      sub(/"$/, "", key)
      key = toupper(key)
      if (key ~ /^[A-Z0-9_]+:[A-Z0-9_]+$/) print key
      next
    }
  ' "$file"
}

for po in share/lang/*.po; do
  if [ "$po" = "$target_file" ]; then
    continue
  fi
  extract_keys "$po"
done | sort -u > "$tmp_base"

extract_keys "$target_file" | sort -u > "$tmp_target"

comm -23 "$tmp_base" "$tmp_target" > "$tmp_missing"

if [ "$count_only" -eq 1 ] && [ "$grouped" -eq 0 ]; then
  wc -l < "$tmp_missing" | tr -d " "
  exit 0
fi

if [ "$count_only" -eq 1 ] && [ "$grouped" -eq 1 ]; then
  awk -F: '/^[A-Z0-9_]+:[A-Z0-9_]+$/ { c[$1]++ } END { for (m in c) print m, c[m] }' "$tmp_missing" | sort
  exit 0
fi

echo "# Missing keys for locale: $target_locale"
echo "# Baseline keys: $(wc -l < "$tmp_base" | tr -d " ")"
echo "# Locale keys:   $(wc -l < "$tmp_target" | tr -d " ")"
echo

if [ "$grouped" -eq 1 ]; then
  awk -F: '
    /^[A-Z0-9_]+:[A-Z0-9_]+$/ {
      if (!(($1) in seen)) {
        order[++n] = $1
        seen[$1] = 1
      }
      keys[$1] = keys[$1] $0 "\n"
    }
    END {
      for (i = 1; i <= n; i++) {
        m = order[i]
        print m ":"
        printf "%s", keys[m]
        print ""
      }
    }
  ' "$tmp_missing"
else
  cat "$tmp_missing"
fi
