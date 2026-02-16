#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Evaluate keep/drop gates for one candidate variant against baseline.

Usage:
  evaluate_gates.sh <run_dir> <baseline_variant> <candidate_variant> [options]

Options:
  --throughput-target-fraction F   Required relative gain (default: 0.10)
  --p95-guardrail-fraction F       Max allowed relative p95 regression (default: 0.05)
  --p99-guardrail-fraction F       Max allowed relative p99 regression (default: 0.05)
  --help                           Show this help
EOF
}

if [ "${1:-}" = "--help" ] || [ $# -lt 3 ]; then
  usage
  exit 0
fi

run_dir="$1"
baseline="$2"
candidate="$3"
shift 3

throughput_target="0.10"
p95_guardrail="0.05"
p99_guardrail="0.05"

while [ $# -gt 0 ]; do
  case "$1" in
    --throughput-target-fraction) throughput_target="$2"; shift 2 ;;
    --p95-guardrail-fraction) p95_guardrail="$2"; shift 2 ;;
    --p99-guardrail-fraction) p99_guardrail="$2"; shift 2 ;;
    --help) usage; exit 0 ;;
    *)
      echo "unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

report="$run_dir/report.json"
if [ ! -f "$report" ]; then
  echo "missing report: $report" >&2
  exit 1
fi

b_tp="$(jq -r --arg v "$baseline" '.aggregate[] | select(.variant==$v) | .median_throughput_jobs_per_s' "$report")"
c_tp="$(jq -r --arg v "$candidate" '.aggregate[] | select(.variant==$v) | .median_throughput_jobs_per_s' "$report")"
b_p95="$(jq -r --arg v "$baseline" '.aggregate[] | select(.variant==$v) | .job_duration_p95_ms' "$report")"
c_p95="$(jq -r --arg v "$candidate" '.aggregate[] | select(.variant==$v) | .job_duration_p95_ms' "$report")"
b_p99="$(jq -r --arg v "$baseline" '.aggregate[] | select(.variant==$v) | .job_duration_p99_ms' "$report")"
c_p99="$(jq -r --arg v "$candidate" '.aggregate[] | select(.variant==$v) | .job_duration_p99_ms' "$report")"
b_integrity="$(jq -r --arg v "$baseline" '.aggregate[] | select(.variant==$v) | .integrity_failures' "$report")"
c_integrity="$(jq -r --arg v "$candidate" '.aggregate[] | select(.variant==$v) | .integrity_failures' "$report")"
b_success="$(jq -r --arg v "$baseline" '.aggregate[] | select(.variant==$v) | .success_rate' "$report")"
c_success="$(jq -r --arg v "$candidate" '.aggregate[] | select(.variant==$v) | .success_rate' "$report")"

if [ -z "$b_tp" ] || [ -z "$c_tp" ]; then
  echo "variant not found in report: baseline=$baseline candidate=$candidate" >&2
  exit 1
fi

tp_delta="$(awk -v a="$b_tp" -v b="$c_tp" 'BEGIN{ if(a==0){print "nan"} else {printf "%.6f", (b-a)/a} }')"
p95_delta="$(awk -v a="$b_p95" -v b="$c_p95" 'BEGIN{ if(a==0){print "nan"} else {printf "%.6f", (b-a)/a} }')"
p99_delta="$(awk -v a="$b_p99" -v b="$c_p99" 'BEGIN{ if(a==0){print "nan"} else {printf "%.6f", (b-a)/a} }')"

tp_gate="$(awk -v d="$tp_delta" -v t="$throughput_target" 'BEGIN{ print (d+0 >= t+0) ? "pass" : "fail" }')"
p95_gate="$(awk -v d="$p95_delta" -v g="$p95_guardrail" 'BEGIN{ print (d+0 <= g+0) ? "pass" : "fail" }')"
p99_gate="$(awk -v d="$p99_delta" -v g="$p99_guardrail" 'BEGIN{ print (d+0 <= g+0) ? "pass" : "fail" }')"
integrity_gate="$(awk -v b="$b_integrity" -v c="$c_integrity" 'BEGIN{ print (b==0 && c==0) ? "pass" : "fail" }')"
success_gate="$(awk -v b="$b_success" -v c="$c_success" 'BEGIN{ print (c+0 >= b+0) ? "pass" : "fail" }')"

overall="drop"
if [ "$tp_gate" = "pass" ] && [ "$p95_gate" = "pass" ] && [ "$p99_gate" = "pass" ] && [ "$integrity_gate" = "pass" ] && [ "$success_gate" = "pass" ]; then
  overall="keep"
fi

echo "run_dir=$run_dir"
echo "baseline=$baseline"
echo "candidate=$candidate"
echo "throughput_target_fraction=$throughput_target"
echo "p95_guardrail_fraction=$p95_guardrail"
echo "p99_guardrail_fraction=$p99_guardrail"
echo
echo "baseline_throughput=$b_tp"
echo "candidate_throughput=$c_tp"
echo "throughput_delta_fraction=$tp_delta ($tp_gate)"
echo
echo "baseline_p95_ms=$b_p95"
echo "candidate_p95_ms=$c_p95"
echo "p95_delta_fraction=$p95_delta ($p95_gate)"
echo
echo "baseline_p99_ms=$b_p99"
echo "candidate_p99_ms=$c_p99"
echo "p99_delta_fraction=$p99_delta ($p99_gate)"
echo
echo "baseline_success_rate=$b_success"
echo "candidate_success_rate=$c_success"
echo "success_gate=$success_gate"
echo "baseline_integrity_failures=$b_integrity"
echo "candidate_integrity_failures=$c_integrity"
echo "integrity_gate=$integrity_gate"
echo
echo "recommendation=$overall"

