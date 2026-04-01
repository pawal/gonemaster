#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Run full Track A-E benchmark workflow with one command.

Workflow:
1) Quick screen on 600 domains for all variants vs baseline.
2) Gate evaluation for every candidate variant.
3) Decision-grade A/B reruns on 1000 domains for survivors (or all).
4) Per-domain outlier tables for decision runs.
5) Consolidated summary output.

Usage:
  run_all_tracks.sh --variants FILE [options]

Required:
  --variants FILE               Variant file with "name ref" per line.
                                Must include baseline variant name (default: main).

Options:
  --baseline NAME              Baseline variant name (default: main)
  --out-root DIR               Output root (default: perf-runs/server)
  --quick-domains FILE         Quick corpus (default: tools/server-perf-kit/corpus/domains-fixed-600.txt)
  --decision-domains FILE      Decision corpus (default: tools/server-perf-kit/corpus/domains-fixed-1000.txt)
  --workers N                  Server workers (default: 8)
  --max-concurrent-jobs N      Engine limiter (default: 8)
  --warmups N                  Warmup runs per variant (default: 1)
  --quick-repeats N            Measured quick repeats (default: 3)
  --decision-repeats N         Measured decision repeats (default: 7)
  --mode MODE                  quick-only | quick-and-decision (default: quick-and-decision)
  --decision-for MODE          survivors | all | none (default: survivors)
  --survivors LIST             Comma/space-separated candidate names to force decision runs
  --throughput-target-fraction F   Gate threshold (default: 0.10)
  --p95-guardrail-fraction F       Gate threshold (default: 0.05)
  --p99-guardrail-fraction F       Gate threshold (default: 0.05)
  --profile FILE               Optional profile file passed to server
  --min-level LEVEL            Server min-level (default: INFO)
  --batch-poll-seconds N       Poll interval (default: 2)
  --sample-seconds N           Sampling interval (default: 1)
  --batch-timeout-seconds N    Run timeout (default: 5400)
  --go-cache DIR               GOCACHE override
  --help                       Show this help
EOF
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

resolve_path() {
  local p="$1"
  case "$p" in
    /*) echo "$p" ;;
    *) echo "$repo_root/$p" ;;
  esac
}

contains_word() {
  local needle="$1"
  shift
  local item
  for item in "$@"; do
    if [ "$item" = "$needle" ]; then
      return 0
    fi
  done
  return 1
}

parse_variants() {
  local file="$1"
  variant_names=()
  variant_refs=()
  while IFS= read -r raw; do
    line="${raw%%#*}"
    line="$(echo "$line" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
    [ -n "$line" ] || continue
    set -- $line
    if [ $# -lt 2 ]; then
      echo "invalid variants line (need: name ref): $raw" >&2
      exit 1
    fi
    variant_names+=("$1")
    variant_refs+=("$2")
  done <"$file"
}

make_candidate_list() {
  candidate_names=()
  local name
  for name in "${variant_names[@]}"; do
    if [ "$name" != "$baseline" ]; then
      candidate_names+=("$name")
    fi
  done
  if [ "${#candidate_names[@]}" -eq 0 ]; then
    echo "no candidate variants (variants file only contains baseline '$baseline')" >&2
    exit 1
  fi
}

gate_eval_one() {
  local run_dir="$1"
  local candidate="$2"
  local gate_file="$3"

  "$evaluate_script" "$run_dir" "$baseline" "$candidate" \
    --throughput-target-fraction "$throughput_target" \
    --p95-guardrail-fraction "$p95_guardrail" \
    --p99-guardrail-fraction "$p99_guardrail" | tee "$gate_file" >/dev/null
}

append_gate_summary_row() {
  local stage="$1"
  local run_dir="$2"
  local candidate="$3"
  local gate_file="$4"
  local report="$run_dir/report.json"

  local b_tp c_tp b_p95 c_p95 b_p99 c_p99
  local tp_delta tp_gate p95_delta p95_gate p99_delta p99_gate success_gate integrity_gate rec

  b_tp="$(jq -r --arg v "$baseline" '.aggregate[] | select(.variant==$v) | .median_throughput_jobs_per_s' "$report")"
  c_tp="$(jq -r --arg v "$candidate" '.aggregate[] | select(.variant==$v) | .median_throughput_jobs_per_s' "$report")"
  b_p95="$(jq -r --arg v "$baseline" '.aggregate[] | select(.variant==$v) | .job_duration_p95_ms' "$report")"
  c_p95="$(jq -r --arg v "$candidate" '.aggregate[] | select(.variant==$v) | .job_duration_p95_ms' "$report")"
  b_p99="$(jq -r --arg v "$baseline" '.aggregate[] | select(.variant==$v) | .job_duration_p99_ms' "$report")"
  c_p99="$(jq -r --arg v "$candidate" '.aggregate[] | select(.variant==$v) | .job_duration_p99_ms' "$report")"

  tp_delta="$(sed -n 's/^throughput_delta_fraction=\([^ ]*\).*/\1/p' "$gate_file" | tail -n 1)"
  tp_gate="$(sed -n 's/^throughput_delta_fraction=[^ ]* (\([^)]*\)).*/\1/p' "$gate_file" | tail -n 1)"
  p95_delta="$(sed -n 's/^p95_delta_fraction=\([^ ]*\).*/\1/p' "$gate_file" | tail -n 1)"
  p95_gate="$(sed -n 's/^p95_delta_fraction=[^ ]* (\([^)]*\)).*/\1/p' "$gate_file" | tail -n 1)"
  p99_delta="$(sed -n 's/^p99_delta_fraction=\([^ ]*\).*/\1/p' "$gate_file" | tail -n 1)"
  p99_gate="$(sed -n 's/^p99_delta_fraction=[^ ]* (\([^)]*\)).*/\1/p' "$gate_file" | tail -n 1)"
  success_gate="$(sed -n 's/^success_gate=\(.*\)$/\1/p' "$gate_file" | tail -n 1)"
  integrity_gate="$(sed -n 's/^integrity_gate=\(.*\)$/\1/p' "$gate_file" | tail -n 1)"
  rec="$(sed -n 's/^recommendation=\(.*\)$/\1/p' "$gate_file" | tail -n 1)"

  echo "${stage},${candidate},${b_tp},${c_tp},${tp_delta},${tp_gate},${b_p95},${c_p95},${p95_delta},${p95_gate},${b_p99},${c_p99},${p99_delta},${p99_gate},${success_gate},${integrity_gate},${rec},${run_dir}" >>"$gate_summary_csv"
}

generate_outlier_table() {
  local run_dir="$1"
  local variant="$2"
  local out_file="$3"
  local top_file="$4"

  local tmp
  tmp="$(mktemp)"
  trap 'rm -f "$tmp"' RETURN

  local matched=0
  local batch_json run_name
  for batch_json in "$run_dir"/runs/"$variant"-run-*/batch_final.json; do
    [ -f "$batch_json" ] || continue
    matched=1
    run_name="$(basename "$(dirname "$batch_json")")"
    jq -r --arg run "$run_name" '
      def ts: sub("\\..*Z$";"Z") | fromdateiso8601;
      .items[]
      | select(.started_at != null and .finished_at != null and .started_at != "" and .finished_at != "")
      | [(((.finished_at|ts)-(.started_at|ts))*1000|floor), .domain, .status, $run] | @tsv
    ' "$batch_json" >>"$tmp"
  done

  {
    echo -e "domain\tcount\tavg_ms\tmax_ms\tnon_success_count"
    if [ "$matched" -eq 1 ] && [ -s "$tmp" ]; then
      awk -F'\t' '
      {
        d=$2
        n[d]++
        s[d]+=$1
        if ($1 > m[d]) m[d]=$1
        if ($3 != "succeeded") f[d]++
      }
      END {
        for (d in n) printf "%s\t%d\t%.2f\t%d\t%d\n", d, n[d], s[d]/n[d], m[d], f[d]+0
      }' "$tmp" | sort -t "$(printf '\t')" -k4,4nr
    fi
  } >"$out_file"

  head -n 51 "$out_file" >"$top_file"
}

baseline="main"
out_root="perf-runs/server"
quick_domains="tools/server-perf-kit/corpus/domains-fixed-600.txt"
decision_domains="tools/server-perf-kit/corpus/domains-fixed-1000.txt"
workers=8
max_concurrent_jobs=8
warmups=1
quick_repeats=3
decision_repeats=7
mode="quick-and-decision"
decision_for="survivors"
survivors_override=""
throughput_target="0.10"
p95_guardrail="0.05"
p99_guardrail="0.05"
profile_file=""
min_level="INFO"
batch_poll_seconds=2
sample_seconds=1
batch_timeout_seconds=5400
go_cache=""
variants_file=""

while [ $# -gt 0 ]; do
  case "$1" in
    --variants) variants_file="$2"; shift 2 ;;
    --baseline) baseline="$2"; shift 2 ;;
    --out-root) out_root="$2"; shift 2 ;;
    --quick-domains) quick_domains="$2"; shift 2 ;;
    --decision-domains) decision_domains="$2"; shift 2 ;;
    --workers) workers="$2"; shift 2 ;;
    --max-concurrent-jobs) max_concurrent_jobs="$2"; shift 2 ;;
    --warmups) warmups="$2"; shift 2 ;;
    --quick-repeats) quick_repeats="$2"; shift 2 ;;
    --decision-repeats) decision_repeats="$2"; shift 2 ;;
    --mode) mode="$2"; shift 2 ;;
    --decision-for) decision_for="$2"; shift 2 ;;
    --survivors) survivors_override="$2"; shift 2 ;;
    --throughput-target-fraction) throughput_target="$2"; shift 2 ;;
    --p95-guardrail-fraction) p95_guardrail="$2"; shift 2 ;;
    --p99-guardrail-fraction) p99_guardrail="$2"; shift 2 ;;
    --profile) profile_file="$2"; shift 2 ;;
    --min-level) min_level="$2"; shift 2 ;;
    --batch-poll-seconds) batch_poll_seconds="$2"; shift 2 ;;
    --sample-seconds) sample_seconds="$2"; shift 2 ;;
    --batch-timeout-seconds) batch_timeout_seconds="$2"; shift 2 ;;
    --go-cache) go_cache="$2"; shift 2 ;;
    --help) usage; exit 0 ;;
    *)
      echo "unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [ -z "$variants_file" ]; then
  usage
  exit 1
fi

case "$mode" in
  quick-only|quick-and-decision) ;;
  *)
    echo "invalid --mode: $mode (expected quick-only or quick-and-decision)" >&2
    exit 1
    ;;
esac

case "$decision_for" in
  survivors|all|none) ;;
  *)
    echo "invalid --decision-for: $decision_for (expected survivors, all, or none)" >&2
    exit 1
    ;;
esac

need_cmd jq
need_cmd awk
need_cmd sed
need_cmd git

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(git -C "$script_dir/../.." rev-parse --show-toplevel)"
run_matrix_script="$script_dir/run_tracks_matrix.sh"
evaluate_script="$script_dir/evaluate_gates.sh"
if [ ! -x "$run_matrix_script" ]; then
  echo "missing executable: $run_matrix_script" >&2
  exit 1
fi
if [ ! -x "$evaluate_script" ]; then
  echo "missing executable: $evaluate_script" >&2
  exit 1
fi

variants_file="$(resolve_path "$variants_file")"
quick_domains="$(resolve_path "$quick_domains")"
decision_domains="$(resolve_path "$decision_domains")"
out_root="$(resolve_path "$out_root")"
if [ -n "$profile_file" ]; then
  profile_file="$(resolve_path "$profile_file")"
fi
if [ -n "$go_cache" ]; then
  go_cache="$(resolve_path "$go_cache")"
fi

if [ ! -f "$variants_file" ]; then
  echo "variants file not found: $variants_file" >&2
  exit 1
fi
if [ ! -f "$quick_domains" ]; then
  echo "quick domains file not found: $quick_domains" >&2
  exit 1
fi
if [ ! -f "$decision_domains" ]; then
  echo "decision domains file not found: $decision_domains" >&2
  exit 1
fi
if [ -n "$profile_file" ] && [ ! -f "$profile_file" ]; then
  echo "profile file not found: $profile_file" >&2
  exit 1
fi

parse_variants "$variants_file"
if ! contains_word "$baseline" "${variant_names[@]}"; then
  echo "baseline '$baseline' not found in variants file" >&2
  exit 1
fi
make_candidate_list

run_ts="$(date -u +%Y%m%d-%H%M%S)"
session_dir="${out_root%/}/all-tracks-$run_ts"
quick_dir="$session_dir/quick"
decision_root="$session_dir/decision"
mkdir -p "$session_dir" "$decision_root"

gate_summary_csv="$session_dir/gate-summary.csv"
echo "stage,candidate,baseline_median_throughput,candidate_median_throughput,throughput_delta_fraction,throughput_gate,baseline_p95_ms,candidate_p95_ms,p95_delta_fraction,p95_gate,baseline_p99_ms,candidate_p99_ms,p99_delta_fraction,p99_gate,success_gate,integrity_gate,recommendation,run_dir" >"$gate_summary_csv"

{
  echo "generated_at_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "baseline=$baseline"
  echo "variants_file=$variants_file"
  echo "quick_domains=$quick_domains"
  echo "decision_domains=$decision_domains"
  echo "workers=$workers"
  echo "max_concurrent_jobs=$max_concurrent_jobs"
  echo "warmups=$warmups"
  echo "quick_repeats=$quick_repeats"
  echo "decision_repeats=$decision_repeats"
  echo "mode=$mode"
  echo "decision_for=$decision_for"
  echo "throughput_target_fraction=$throughput_target"
  echo "p95_guardrail_fraction=$p95_guardrail"
  echo "p99_guardrail_fraction=$p99_guardrail"
} >"$session_dir/run-config.env"

echo "== quick screen =="
quick_args=(
  --variants "$variants_file"
  --domains "$quick_domains"
  --workers "$workers"
  --max-concurrent-jobs "$max_concurrent_jobs"
  --warmups "$warmups"
  --repeats "$quick_repeats"
  --out-dir "$quick_dir"
  --min-level "$min_level"
  --batch-poll-seconds "$batch_poll_seconds"
  --sample-seconds "$sample_seconds"
  --batch-timeout-seconds "$batch_timeout_seconds"
)
if [ -n "$profile_file" ]; then
  quick_args+=(--profile "$profile_file")
fi
if [ -n "$go_cache" ]; then
  quick_args+=(--go-cache "$go_cache")
fi
"$run_matrix_script" "${quick_args[@]}"

survivors=()
echo "== quick gate evaluation =="
for candidate in "${candidate_names[@]}"; do
  gate_file="$quick_dir/gate-$candidate.txt"
  gate_eval_one "$quick_dir" "$candidate" "$gate_file"
  append_gate_summary_row "quick" "$quick_dir" "$candidate" "$gate_file"
  rec="$(sed -n 's/^recommendation=\(.*\)$/\1/p' "$gate_file" | tail -n 1)"
  if [ "$rec" = "keep" ]; then
    survivors+=("$candidate")
  fi
done

decision_candidates=()
if [ "$mode" = "quick-and-decision" ] && [ "$decision_for" != "none" ]; then
  if [ -n "$survivors_override" ]; then
    override_norm="$(echo "$survivors_override" | tr ',' ' ')"
    for cand in $override_norm; do
      if ! contains_word "$cand" "${candidate_names[@]}"; then
        echo "override survivor is not a candidate variant: $cand" >&2
        exit 1
      fi
      if ! contains_word "$cand" "${decision_candidates[@]}"; then
        decision_candidates+=("$cand")
      fi
    done
  elif [ "$decision_for" = "all" ]; then
    decision_candidates=("${candidate_names[@]}")
  else
    decision_candidates=("${survivors[@]}")
  fi
fi

if [ "${#decision_candidates[@]}" -gt 0 ]; then
  echo "== decision-grade runs =="
  for candidate in "${decision_candidates[@]}"; do
    ref="$(awk -v c="$candidate" '
      {
        line=$0
        sub(/#.*/, "", line)
        gsub(/^[ \t]+|[ \t]+$/, "", line)
        if (line == "") next
        split(line, a, /[ \t]+/)
        if (a[1] == c) { print a[2]; exit }
      }' "$variants_file")"
    if [ -z "$ref" ]; then
      echo "could not resolve ref for candidate: $candidate" >&2
      exit 1
    fi

    candidate_variants="$session_dir/variants-${candidate}.tsv"
    {
      echo "$baseline $baseline"
      echo "$candidate $ref"
    } >"$candidate_variants"

    run_dir="$decision_root/$candidate"
    decision_args=(
      --variants "$candidate_variants"
      --domains "$decision_domains"
      --workers "$workers"
      --max-concurrent-jobs "$max_concurrent_jobs"
      --warmups "$warmups"
      --repeats "$decision_repeats"
      --out-dir "$run_dir"
      --min-level "$min_level"
      --batch-poll-seconds "$batch_poll_seconds"
      --sample-seconds "$sample_seconds"
      --batch-timeout-seconds "$batch_timeout_seconds"
    )
    if [ -n "$profile_file" ]; then
      decision_args+=(--profile "$profile_file")
    fi
    if [ -n "$go_cache" ]; then
      decision_args+=(--go-cache "$go_cache")
    fi
    "$run_matrix_script" "${decision_args[@]}"

    gate_file="$run_dir/gate-$candidate.txt"
    gate_eval_one "$run_dir" "$candidate" "$gate_file"
    append_gate_summary_row "decision" "$run_dir" "$candidate" "$gate_file"

    generate_outlier_table "$run_dir" "$baseline" "$run_dir/outliers-${baseline}.tsv" "$run_dir/outliers-${baseline}-top50.tsv"
    generate_outlier_table "$run_dir" "$candidate" "$run_dir/outliers-${candidate}.tsv" "$run_dir/outliers-${candidate}-top50.tsv"
  done
fi

summary_md="$session_dir/SUMMARY.md"
{
  echo "# All Tracks Perf Workflow"
  echo
  echo "- Generated: \`$(date -u +%Y-%m-%dT%H:%M:%SZ)\`"
  echo "- Baseline: \`$baseline\`"
  echo "- Session dir: \`$session_dir\`"
  echo "- Quick run: \`$quick_dir\`"
  if [ "${#decision_candidates[@]}" -gt 0 ]; then
    echo "- Decision runs root: \`$decision_root\`"
  else
    echo "- Decision runs: skipped"
  fi
  echo
  echo "## Gate Summary"
  echo
  echo "| Stage | Candidate | Throughput delta | p95 delta | p99 delta | Success gate | Integrity gate | Recommendation | Run dir |"
  echo "| --- | --- | ---: | ---: | ---: | --- | --- | --- | --- |"
  awk -F',' 'NR>1 {
    printf "| %s | %s | %s (%s) | %s (%s) | %s (%s) | %s | %s | %s | `%s` |\n",
      $1, $2, $5, $6, $9, $10, $13, $14, $15, $16, $17, $18
  }' "$gate_summary_csv"
  echo
  echo "## Outputs"
  echo
  echo "- Gate summary csv: \`$gate_summary_csv\`"
  echo "- Run config: \`$session_dir/run-config.env\`"
  if [ "${#decision_candidates[@]}" -gt 0 ]; then
    echo "- Per-domain outlier tables:"
    for candidate in "${decision_candidates[@]}"; do
      echo "  - \`$decision_root/$candidate/outliers-${baseline}.tsv\`"
      echo "  - \`$decision_root/$candidate/outliers-${candidate}.tsv\`"
    done
  fi
} >"$summary_md"

echo "completed: $session_dir"
echo "summary:   $summary_md"
echo "gates:     $gate_summary_csv"
