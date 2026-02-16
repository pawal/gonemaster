#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Create a deterministic domain corpus from Majestic Million.

Usage:
  make_domains_from_majestic.sh --input FILE --out FILE [options]

Options:
  --input FILE      Majestic CSV or plain domain list (required)
  --out FILE        Output domain list (required)
  --count N         Output size (default: 1000)
  --seed S          Deterministic seed string (default: 20260216)
  --canary FILE     Canary/outlier domains file (default: tools/server-perf-kit/canary-domains.txt)
  --help            Show this help

Behavior:
  - If rank+domain CSV is detected, uses stratified sampling:
    70%: rank 1..100000
    20%: rank 100001..500000
    10%: rank 500001..1000000
  - Otherwise falls back to deterministic global sampling.
  - Normalizes all entries to registrable domains (public-suffix aware).
  - Force-includes canary/outlier domains (also registrable-normalized).
EOF
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

sample_deterministic() {
  local infile="$1"
  local n="$2"
  local seed="$3"
  local label="$4"
  local outfile="$5"
  local seed_num
  local total

  total="$(wc -l <"$infile" | awk '{print $1}')"
  if [ "$n" -le 0 ] || [ "$total" -le 0 ]; then
    : >"$outfile"
    return
  fi
  if [ "$total" -le "$n" ]; then
    cp "$infile" "$outfile"
    return
  fi

  if command -v shuf >/dev/null 2>&1; then
    local rs
    rs="$(mktemp)"
    # `yes` and `tr` receive SIGPIPE when `head` has enough bytes; ignore that.
    yes "${seed}-${label}" | tr -d '\n' | head -c 1048576 >"$rs" || true
    shuf --random-source="$rs" -n "$n" "$infile" >"$outfile"
    rm -f "$rs"
    return
  fi

  # Portable fallback when GNU shuf is unavailable.
  seed_num="$(printf '%s' "${seed}-${label}" | cksum | awk '{print $1}')"
  awk -v seed="$seed_num" '
    BEGIN { srand(seed) }
    { printf "%.12f\t%s\n", rand(), $0 }
  ' "$infile" | sort -k1,1n | cut -f2- | awk -v n="$n" 'NR<=n { print }' >"$outfile"
}

input=""
out=""
count=1000
seed="20260216"
canary_file=""

while [ $# -gt 0 ]; do
  case "$1" in
    --input) input="$2"; shift 2 ;;
    --out) out="$2"; shift 2 ;;
    --count) count="$2"; shift 2 ;;
    --seed) seed="$2"; shift 2 ;;
    --canary) canary_file="$2"; shift 2 ;;
    --help) usage; exit 0 ;;
    *)
      echo "unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [ -z "$input" ] || [ -z "$out" ]; then
  usage
  exit 1
fi
if [ ! -f "$input" ]; then
  echo "input not found: $input" >&2
  exit 1
fi

need_cmd awk
need_cmd sha256sum
need_cmd go

tmp_root="$(mktemp -d)"
trap 'rm -rf "$tmp_root"' EXIT

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
registrable_pkg="$repo_root/tools/server-perf-kit/cmd/registrable"
if [ -z "$canary_file" ]; then
  canary_file="$repo_root/tools/server-perf-kit/canary-domains.txt"
fi
if [ ! -f "$registrable_pkg/main.go" ]; then
  echo "registrable helper not found: $registrable_pkg/main.go" >&2
  exit 1
fi
go_cache="${GOCACHE:-$tmp_root/gocache}"
mkdir -p "$go_cache"

normalize_registrable() {
  local infile="$1"
  local outfile="$2"
  (
    cd "$repo_root"
    GOCACHE="$go_cache" go run ./tools/server-perf-kit/cmd/registrable <"$infile" >"$outfile"
  )
}

domains_all="$tmp_root/domains_all.txt"
ranked="$tmp_root/ranked.tsv"

# Try rank+domain extraction for Majestic CSV-like input.
awk -F',' '
NR==1 {
  header=tolower($0)
  maybe_csv = (index(header, "globalrank") > 0 || index(header, "domain") > 0)
}
NR>1 {
  rank=$1
  dom=$3
  gsub(/"/, "", rank)
  gsub(/"/, "", dom)
  gsub(/\r/, "", dom)
  dom=tolower(dom)
  if (rank ~ /^[0-9]+$/ && dom != "") print rank "\t" dom
}
' "$input" >"$ranked"

# Convert to registrable domains while preserving rank when available.
ranked_norm="$tmp_root/ranked_norm.tsv"
if [ -s "$ranked" ]; then
  normalize_registrable "$ranked" "$ranked_norm"
fi
ranked_lines="$(wc -l <"$ranked_norm" 2>/dev/null | awk '{print $1}')"

if [ "$ranked_lines" -gt 0 ]; then
  # Use ranked mode; keep best (lowest) rank per registrable domain.
  awk -F'\t' '
    {
      r = $1 + 0
      d = $2
      if (d == "") next
      if (!(d in min) || r < min[d]) min[d] = r
    }
    END {
      for (d in min) print min[d] "\t" d
    }
  ' "$ranked_norm" | sort -t "$(printf '\t')" -k1,1n >"$tmp_root/ranked_uniq.tsv"

  awk -F'\t' '$1 >= 1 && $1 <= 100000 {print $2}' "$tmp_root/ranked_uniq.tsv" >"$tmp_root/tier1.txt"
  awk -F'\t' '$1 >= 100001 && $1 <= 500000 {print $2}' "$tmp_root/ranked_uniq.tsv" >"$tmp_root/tier2.txt"
  awk -F'\t' '$1 >= 500001 && $1 <= 1000000 {print $2}' "$tmp_root/ranked_uniq.tsv" >"$tmp_root/tier3.txt"
  awk -F'\t' '{print $2}' "$tmp_root/ranked_uniq.tsv" >"$domains_all"

  c1=$((count * 70 / 100))
  c2=$((count * 20 / 100))
  c3=$((count - c1 - c2))

  sample_deterministic "$tmp_root/tier1.txt" "$c1" "$seed" "tier1" "$tmp_root/s1.txt"
  sample_deterministic "$tmp_root/tier2.txt" "$c2" "$seed" "tier2" "$tmp_root/s2.txt"
  sample_deterministic "$tmp_root/tier3.txt" "$c3" "$seed" "tier3" "$tmp_root/s3.txt"
  cat "$tmp_root/s1.txt" "$tmp_root/s2.txt" "$tmp_root/s3.txt" | awk '!seen[$0]++' >"$tmp_root/selected.txt"
else
  # Fallback plain list extraction.
  awk '
  {
    line=$0
    gsub(/\r/, "", line)
    if (NR==1 && index(line, ",")>0) next
    if (index(line, ",")>0) {
      n=split(line, a, ",")
      dom=a[n]
      gsub(/"/, "", dom)
      dom=tolower(dom)
      if (dom != "") print dom
    } else {
      dom=tolower(line)
      if (dom != "") print dom
    }
  }' "$input" >"$tmp_root/fallback_raw.txt"
  normalize_registrable "$tmp_root/fallback_raw.txt" "$tmp_root/fallback_norm.txt"
  awk '!seen[$0]++' "$tmp_root/fallback_norm.txt" >"$domains_all"
  sample_deterministic "$domains_all" "$count" "$seed" "fallback" "$tmp_root/selected.txt"
fi

if [ ! -s "$domains_all" ]; then
  echo "could not extract domains from input: $input" >&2
  exit 1
fi

# Ensure canary inclusion.
if [ -f "$canary_file" ]; then
  awk 'NF{gsub(/\r/,""); print tolower($0)}' "$canary_file" >"$tmp_root/canary_raw.txt"
  normalize_registrable "$tmp_root/canary_raw.txt" "$tmp_root/canary_norm.txt"
  awk '!seen[$0]++' "$tmp_root/canary_norm.txt" >"$tmp_root/canary.txt"
else
  : >"$tmp_root/canary.txt"
fi

cp "$tmp_root/selected.txt" "$tmp_root/with_canary.txt"
while IFS= read -r d; do
  [ -n "$d" ] || continue
  if ! grep -qx "$d" "$tmp_root/with_canary.txt"; then
    echo "$d" >>"$tmp_root/with_canary.txt"
  fi
done <"$tmp_root/canary.txt"

# Trim/pad to exact count while preserving canaries.
awk '!seen[$0]++' "$tmp_root/with_canary.txt" >"$tmp_root/uniq.txt"
canary_count="$(wc -l <"$tmp_root/canary.txt" | awk '{print $1}')"
if [ "$canary_count" -gt "$count" ]; then
  echo "canary count ($canary_count) exceeds requested output count ($count)" >&2
  exit 1
fi
current="$(wc -l <"$tmp_root/uniq.txt" | awk '{print $1}')"
if [ "$current" -lt "$count" ]; then
  comm -23 <(sort "$domains_all") <(sort "$tmp_root/uniq.txt") >"$tmp_root/remainder.txt"
  need=$((count - current))
  sample_deterministic "$tmp_root/remainder.txt" "$need" "$seed" "remainder" "$tmp_root/fill.txt"
  cat "$tmp_root/uniq.txt" "$tmp_root/fill.txt" | awk '!seen[$0]++' >"$tmp_root/final.txt"
else
  cp "$tmp_root/uniq.txt" "$tmp_root/final.txt"
fi

# If still above target, drop non-canaries from the end.
awk '!seen[$0]++' "$tmp_root/canary.txt" >"$tmp_root/cset.txt"
while [ "$(wc -l <"$tmp_root/final.txt" | awk '{print $1}')" -gt "$count" ]; do
  victim="$(
    awk '
      NR==FNR { c[$0]=1; next }
      { a[NR]=$0; n=NR }
      END {
        for (i=n; i>=1; i--) {
          if (!(a[i] in c)) { print a[i]; exit }
        }
      }' "$tmp_root/cset.txt" "$tmp_root/final.txt"
  )"
  [ -n "$victim" ] || break
  grep -vx "$victim" "$tmp_root/final.txt" >"$tmp_root/final.new"
  mv "$tmp_root/final.new" "$tmp_root/final.txt"
done

head -n "$count" "$tmp_root/final.txt" >"$out"

out_count="$(wc -l <"$out" | awk '{print $1}')"
out_sha="$(sha256sum "$out" | awk '{print $1}')"
echo "wrote $out_count domains to $out"
echo "sha256=$out_sha"
