#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Build a nameserver-concentrated domain corpus from a zone file.

Where make_domains_from_majestic.sh produces an NS-diverse corpus, this one
produces the opposite: every domain in the output delegates to the same small
nameserver set. That concentration is what makes a farm's rate limiter
observable, since a diverse corpus spreads our query volume too thin to trip
anything.

Usage:
  make_domains_by_nameserver.sh --zone FILE --ns-pattern REGEX --out FILE [options]

Required:
  --zone FILE          Zone file in presentation format (dig AXFR output works)
  --ns-pattern REGEX   POSIX ERE matched against each NS target, without the
                       trailing dot, e.g. '^ns0[12]\.one\.com$'
  --out FILE           Output domain list

Options:
  --count N            Output size (default: 500; 0 keeps every match)
  --seed S             Deterministic seed string (default: 20260812)
  --min-ns N           Require at least N matching NS records per domain
                       (default: 1)
  --manifest FILE      Write a JSON manifest beside the list (default:
                       <out>.manifest.json)
  --help               Show this help

Guardrails, all deliberate:
  - Only names directly under the zone apex are eligible, so a delegation
    below a delegation cannot inflate the corpus.
  - The domain lists themselves are not committed: they are regenerable from
    zone data, and a published list of one operator's customers is a target
    list nobody needs. Commit this script and the manifest instead.
  - Zone data from zonedata.iis.se is CC BY 4.0; keep the attribution in the
    manifest with the results.
EOF
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

# Deterministic sample, seeded so a corpus can be rebuilt exactly. Same
# approach as make_domains_from_majestic.sh, minus the rank tiers.
sample_deterministic() {
  local infile="$1"
  local n="$2"
  local seed="$3"
  local outfile="$4"
  local total seed_num

  total="$(wc -l <"$infile" | awk '{print $1}')"
  if [ "$n" -le 0 ] || [ "$total" -le "$n" ]; then
    cp "$infile" "$outfile"
    return
  fi

  if command -v shuf >/dev/null 2>&1; then
    local rs
    rs="$(mktemp)"
    yes "$seed" | tr -d '\n' | head -c 1048576 >"$rs" || true
    shuf --random-source="$rs" -n "$n" "$infile" | sort >"$outfile"
    rm -f "$rs"
    return
  fi

  seed_num="$(printf '%s' "$seed" | cksum | awk '{print $1}')"
  awk -v seed="$seed_num" '
    BEGIN { srand(seed) }
    { printf "%.12f\t%s\n", rand(), $0 }
  ' "$infile" | sort -k1,1n | cut -f2- | awk -v n="$n" 'NR<=n { print }' | sort >"$outfile"
}

zone_file=""
ns_pattern=""
out=""
count=500
seed="20260812"
min_ns=1
manifest=""

while [ $# -gt 0 ]; do
  case "$1" in
    --zone) zone_file="$2"; shift 2 ;;
    --ns-pattern) ns_pattern="$2"; shift 2 ;;
    --out) out="$2"; shift 2 ;;
    --count) count="$2"; shift 2 ;;
    --seed) seed="$2"; shift 2 ;;
    --min-ns) min_ns="$2"; shift 2 ;;
    --manifest) manifest="$2"; shift 2 ;;
    --help) usage; exit 0 ;;
    *)
      echo "unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [ -z "$zone_file" ] || [ -z "$ns_pattern" ] || [ -z "$out" ]; then
  usage
  exit 1
fi
if [ ! -f "$zone_file" ]; then
  echo "zone file not found: $zone_file" >&2
  exit 1
fi

need_cmd awk
need_cmd sha256sum
need_cmd sort
need_cmd jq

if [ -z "$manifest" ]; then
  manifest="$out.manifest.json"
fi

tmp_root="$(mktemp -d)"
trap 'rm -rf "$tmp_root"' EXIT

# Derive the apex from the zone's own SOA so "one label below the apex" can
# be enforced without the caller having to state the zone name.
apex="$(awk '
  $4 == "SOA" && $1 != "" { print tolower($1); exit }
' "$zone_file")"
if [ -z "$apex" ]; then
  echo "no SOA record found in $zone_file; is it a zone file?" >&2
  exit 1
fi
apex="${apex%.}"

matches="$tmp_root/matches.txt"
awk -v pattern="$ns_pattern" -v apex="$apex" '
  $4 != "NS" { next }
  {
    owner = tolower($1)
    sub(/\.$/, "", owner)
    target = tolower($5)
    sub(/\.$/, "", target)
    if (owner == "" || owner == apex) next
    # Only direct children of the apex: a deeper delegation belongs to a
    # subzone, not to a registrable domain in this zone.
    rest = owner
    suffix = "." apex
    if (index(rest, suffix) != length(rest) - length(suffix) + 1) next
    label = substr(rest, 1, length(rest) - length(suffix))
    if (index(label, ".") > 0) next
    if (target ~ pattern) print owner
  }
' "$zone_file" | sort | uniq -c | awk -v m="$min_ns" '$1 >= m { print $2 }' >"$matches"

match_count="$(wc -l <"$matches" | awk '{print $1}')"
if [ "$match_count" -le 0 ]; then
  echo "no domains in $zone_file delegate to a nameserver matching $ns_pattern" >&2
  exit 1
fi

sample_deterministic "$matches" "$count" "$seed" "$out"

out_count="$(wc -l <"$out" | awk '{print $1}')"
out_sha="$(sha256sum "$out" | awk '{print $1}')"
zone_sha="$(sha256sum "$zone_file" | awk '{print $1}')"
zone_serial="$(awk '$4 == "SOA" { print $7; exit }' "$zone_file")"

jq -n \
  --arg generated_at_utc "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg apex "$apex" \
  --arg ns_pattern "$ns_pattern" \
  --arg zone_file "$zone_file" \
  --arg zone_sha256 "$zone_sha" \
  --arg zone_serial "${zone_serial:-unknown}" \
  --arg seed "$seed" \
  --arg out "$out" \
  --arg out_sha256 "$out_sha" \
  --argjson out_count "$out_count" \
  --argjson candidates "$match_count" \
  --argjson min_ns "$min_ns" \
  '{
    generated_at_utc: $generated_at_utc,
    zone_apex: $apex,
    ns_pattern: $ns_pattern,
    min_ns_matches: $min_ns,
    zone_file: $zone_file,
    zone_sha256: $zone_sha256,
    zone_serial: $zone_serial,
    seed: $seed,
    candidates_matched: $candidates,
    output_file: $out,
    output_count: $out_count,
    output_sha256: $out_sha256,
    attribution: "Zone data from zonedata.iis.se, licensed CC BY 4.0.",
    policy: "Domain lists are not committed; regenerate from zone data using this manifest."
  }' >"$manifest"

echo "matched $match_count domains, wrote $out_count to $out"
echo "sha256=$out_sha"
echo "manifest=$manifest"
