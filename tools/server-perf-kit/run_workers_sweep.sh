#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Run a main-branch workers sweep using the fixed perf corpus.

This script executes repeated single-variant runs for main with different
--workers settings and writes an aggregated sweep summary.

Usage:
  run_workers_sweep.sh [options]

Options:
  --main-ref REF                    Git ref for baseline (default: main)
  --domains FILE                    Domains file (default: tools/server-perf-kit/corpus/domains-fixed-1000.txt)
  --out-root DIR                    Output root (default: perf-runs/server)
  --session-name NAME               Session directory name (default: workers-sweep-<utc>)
  --workers-list LIST               Comma/space-separated workers list (default: 4,8,12,16,24,32,48,64)
  --mode MODE                       coupled | fixed (default: coupled)
  --fixed-max-concurrent-jobs N     Required when --mode=fixed
  --warmups N                       Warmups per point (default: 1)
  --repeats N                       Measured repeats per point (default: 3)
  --port N                          Listen port per run (default: 18080)
  --profile FILE                    Optional server profile
  --min-level LEVEL                 Server min-level (default: INFO)
  --batch-poll-seconds N            Poll interval (default: 2)
  --sample-seconds N                Sampling interval (default: 1)
  --batch-timeout-seconds N         Timeout per run (default: 5400)
  --go-cache DIR                    Optional GOCACHE override
  --keep-worktrees                  Keep temporary worktrees for each point
  --help                            Show this help
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

workers_tokens() {
  local raw="$1"
  local token
  for token in $(echo "$raw" | tr ',' ' '); do
    [ -n "$token" ] || continue
    if ! [[ "$token" =~ ^[0-9]+$ ]]; then
      echo "invalid worker value: $token" >&2
      exit 1
    fi
    echo "$token"
  done
}

main_ref="main"
domains_file="tools/server-perf-kit/corpus/domains-fixed-1000.txt"
out_root="perf-runs/server"
session_name=""
workers_list="4,8,12,16,24,32,48,64"
mode="coupled"
fixed_max_concurrent_jobs=""
warmups=1
repeats=3
port=18080
profile_file=""
min_level="INFO"
batch_poll_seconds=2
sample_seconds=1
batch_timeout_seconds=5400
go_cache=""
keep_worktrees=0

while [ $# -gt 0 ]; do
  case "$1" in
    --main-ref) main_ref="$2"; shift 2 ;;
    --domains) domains_file="$2"; shift 2 ;;
    --out-root) out_root="$2"; shift 2 ;;
    --session-name) session_name="$2"; shift 2 ;;
    --workers-list) workers_list="$2"; shift 2 ;;
    --mode) mode="$2"; shift 2 ;;
    --fixed-max-concurrent-jobs) fixed_max_concurrent_jobs="$2"; shift 2 ;;
    --warmups) warmups="$2"; shift 2 ;;
    --repeats) repeats="$2"; shift 2 ;;
    --port) port="$2"; shift 2 ;;
    --profile) profile_file="$2"; shift 2 ;;
    --min-level) min_level="$2"; shift 2 ;;
    --batch-poll-seconds) batch_poll_seconds="$2"; shift 2 ;;
    --sample-seconds) sample_seconds="$2"; shift 2 ;;
    --batch-timeout-seconds) batch_timeout_seconds="$2"; shift 2 ;;
    --go-cache) go_cache="$2"; shift 2 ;;
    --keep-worktrees) keep_worktrees=1; shift ;;
    --help) usage; exit 0 ;;
    *)
      echo "unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

case "$mode" in
  coupled|fixed) ;;
  *)
    echo "invalid --mode: $mode (expected: coupled|fixed)" >&2
    exit 1
    ;;
esac

if [ "$mode" = "fixed" ] && [ -z "$fixed_max_concurrent_jobs" ]; then
  echo "--fixed-max-concurrent-jobs is required when --mode=fixed" >&2
  exit 1
fi

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
repo_root="$(git -C "$script_dir" rev-parse --show-toplevel)"
run_matrix_script="$script_dir/run_tracks_matrix.sh"

need_cmd git
need_cmd jq
need_cmd awk
need_cmd sed
need_cmd date
need_cmd sha256sum

if [ ! -x "$run_matrix_script" ]; then
  echo "missing executable: $run_matrix_script" >&2
  exit 1
fi

domains_file="$(resolve_path "$domains_file")"
if [ ! -f "$domains_file" ]; then
  echo "domains file not found: $domains_file" >&2
  exit 1
fi

profile_abs=""
if [ -n "$profile_file" ]; then
  profile_abs="$(resolve_path "$profile_file")"
  if [ ! -f "$profile_abs" ]; then
    echo "profile file not found: $profile_abs" >&2
    exit 1
  fi
fi

go_cache_abs=""
if [ -n "$go_cache" ]; then
  go_cache_abs="$(resolve_path "$go_cache")"
  mkdir -p "$go_cache_abs"
fi

out_root_abs="$(resolve_path "$out_root")"
mkdir -p "$out_root_abs"

run_ts="$(date -u +%Y%m%d-%H%M%S)"
if [ -z "$session_name" ]; then
  session_name="workers-sweep-$run_ts"
fi
session_dir="$out_root_abs/$session_name"
points_dir="$session_dir/points"
mkdir -p "$points_dir"

main_commit="$(git -C "$repo_root" rev-parse "$main_ref")"
domains_sha="$(sha256sum "$domains_file" | awk '{print $1}')"
domains_count="$(awk 'NF { n++ } END { print n+0 }' "$domains_file")"

host_cores="unknown"
if command -v nproc >/dev/null 2>&1; then
  host_cores="$(nproc)"
elif command -v getconf >/dev/null 2>&1; then
  host_cores="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo unknown)"
fi

variants_file="$session_dir/variants-main.tsv"
echo "main $main_ref" >"$variants_file"

summary_csv="$session_dir/workers-sweep-summary.csv"
echo "workers,max_concurrent_jobs,domains_count,median_throughput_jobs_per_s,median_wall_seconds,p50_ms,p95_ms,p99_ms,success_rate,failed_rate,integrity_failures,median_queue_depth_max,median_in_flight_max,mean_queue_depth_avg,mean_in_flight_avg,mean_cpu_percent_avg,mean_rss_kb_avg,run_dir" >"$summary_csv"

{
  echo "generated_at_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "repo_root=$repo_root"
  echo "main_ref=$main_ref"
  echo "main_commit=$main_commit"
  echo "domains_file=$domains_file"
  echo "domains_sha256=$domains_sha"
  echo "domains_count=$domains_count"
  echo "host_cores=$host_cores"
  echo "workers_list=$workers_list"
  echo "mode=$mode"
  echo "fixed_max_concurrent_jobs=$fixed_max_concurrent_jobs"
  echo "warmups=$warmups"
  echo "repeats=$repeats"
  echo "port=$port"
  echo "batch_poll_seconds=$batch_poll_seconds"
  echo "sample_seconds=$sample_seconds"
  echo "batch_timeout_seconds=$batch_timeout_seconds"
} >"$session_dir/run-config.env"

mapfile -t workers_arr < <(workers_tokens "$workers_list")
if [ "${#workers_arr[@]}" -eq 0 ]; then
  echo "workers list resolved to empty set" >&2
  exit 1
fi

for workers in "${workers_arr[@]}"; do
  max_concurrent_jobs="$workers"
  if [ "$mode" = "fixed" ]; then
    max_concurrent_jobs="$fixed_max_concurrent_jobs"
  fi

  point_tag="workers-${workers}-concurrency-${max_concurrent_jobs}"
  point_dir="$points_dir/$point_tag"
  worktree_root="$point_dir/worktrees"
  mkdir -p "$point_dir" "$worktree_root"

  echo
  echo "== workers sweep point: workers=$workers max-concurrent-jobs=$max_concurrent_jobs =="

  cmd=(
    "$run_matrix_script"
    --variants "$variants_file"
    --domains "$domains_file"
    --out-dir "$point_dir"
    --warmups "$warmups"
    --repeats "$repeats"
    --workers "$workers"
    --max-concurrent-jobs "$max_concurrent_jobs"
    --port "$port"
    --min-level "$min_level"
    --batch-poll-seconds "$batch_poll_seconds"
    --sample-seconds "$sample_seconds"
    --batch-timeout-seconds "$batch_timeout_seconds"
    --worktree-root "$worktree_root"
  )
  if [ -n "$profile_abs" ]; then
    cmd+=(--profile "$profile_abs")
  fi
  if [ -n "$go_cache_abs" ]; then
    cmd+=(--go-cache "$go_cache_abs")
  fi
  if [ "$keep_worktrees" -eq 1 ]; then
    cmd+=(--keep-worktrees)
  fi
  "${cmd[@]}"

  report_json="$point_dir/report.json"
  if [ ! -f "$report_json" ]; then
    echo "missing report.json: $report_json" >&2
    exit 1
  fi

  metrics_tsv="$(
    jq -r '
      (.aggregate[] | select(.variant=="main")) as $a
      | ([.runs[] | select(.variant=="main" and .warmup==false) | .queue_depth_avg]) as $qavg
      | ([.runs[] | select(.variant=="main" and .warmup==false) | .in_flight_avg]) as $iavg
      | ([.runs[] | select(.variant=="main" and .warmup==false) | .server_cpu_percent_avg]) as $cavg
      | ([.runs[] | select(.variant=="main" and .warmup==false) | .server_rss_kb_avg]) as $ravg
      | [
          $a.median_throughput_jobs_per_s,
          $a.median_wall_seconds,
          $a.job_duration_p50_ms,
          $a.job_duration_p95_ms,
          $a.job_duration_p99_ms,
          $a.success_rate,
          $a.failed_rate,
          $a.integrity_failures,
          $a.median_queue_depth_max,
          $a.median_in_flight_max,
          $a.median_cpu_percent_max,
          $a.median_rss_kb_max,
          (if ($qavg|length)==0 then 0 else (($qavg|add)/($qavg|length)) end),
          (if ($iavg|length)==0 then 0 else (($iavg|add)/($iavg|length)) end),
          (if ($cavg|length)==0 then 0 else (($cavg|add)/($cavg|length)) end),
          (if ($ravg|length)==0 then 0 else (($ravg|add)/($ravg|length)) end)
        ]
      | @tsv
    ' "$report_json"
  )"

  IFS=$'\t' read -r tp wall p50 p95 p99 success failed integrity qmax ifmax cpumax rssmax qavg iavg cavg ravg <<<"$metrics_tsv"

  run_dir_rel="${point_dir#$repo_root/}"
  if [ "$run_dir_rel" = "$point_dir" ]; then
    run_dir_rel="$point_dir"
  fi

  echo "$workers,$max_concurrent_jobs,$domains_count,$tp,$wall,$p50,$p95,$p99,$success,$failed,$integrity,$qmax,$ifmax,$qavg,$iavg,$cavg,$ravg,$run_dir_rel" >>"$summary_csv"
done

summary_md="$session_dir/SUMMARY.md"
baseline_tp="$(awk -F',' 'NR==2{print $4}' "$summary_csv")"
baseline_wall="$(awk -F',' 'NR==2{print $5}' "$summary_csv")"
best_tp_line="$(awk -F',' 'NR==2 || ($4+0) > best { best=$4+0; line=$0 } END{ print line }' "$summary_csv")"
best_wall_line="$(awk -F',' 'NR==2 || ($5+0) < best { best=$5+0; line=$0 } END{ print line }' "$summary_csv")"

{
  echo "# Main Branch Workers Sweep"
  echo
  echo "- Generated: \`$(date -u +%Y-%m-%dT%H:%M:%SZ)\`"
  echo "- Main ref/commit: \`$main_ref\` / \`$main_commit\`"
  echo "- Domains: \`$domains_file\` (\`$domains_count\`, sha256 \`$domains_sha\`)"
  echo "- Host cores: \`$host_cores\`"
  echo "- Warmups/repeats per point: \`$warmups/$repeats\`"
  echo "- Sweep mode: \`$mode\`"
  if [ "$mode" = "fixed" ]; then
    echo "- Fixed max-concurrent-jobs: \`$fixed_max_concurrent_jobs\`"
  fi
  echo
  echo "## Results"
  echo
  echo "| workers | max-concurrent-jobs | median throughput (jobs/s) | delta vs first | median wall (s) | delta vs first | p95 (ms) | p99 (ms) | mean in-flight | mean CPU | integrity failures | run dir |"
  echo "| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |"
  awk -F',' -v btp="$baseline_tp" -v bwall="$baseline_wall" '
    NR==1 { next }
    {
      dtp = 0;
      dwall = 0;
      if (btp+0 > 0) dtp = (($4+0) - (btp+0)) / (btp+0) * 100;
      if (bwall+0 > 0) dwall = (($5+0) - (bwall+0)) / (bwall+0) * 100;
      printf "| %d | %d | %.9f | %+0.2f%% | %.6f | %+0.2f%% | %d | %d | %.3f | %.3f | %d | `%s` |\n",
        $1, $2, $4, dtp, $5, dwall, $7, $8, $15, $16, $11, $18
    }
  ' "$summary_csv"
  echo
  if [ -n "$best_tp_line" ]; then
    IFS=',' read -r bw bcm _ btpv bwallv _ bp95 bp99 _ _ binteg _ _ _ _ _ _ brun <<<"$best_tp_line"
    echo "- Best throughput point: workers=\`$bw\`, max-concurrent-jobs=\`$bcm\`, throughput=\`$btpv\`, wall=\`$bwallv\`, p95/p99=\`$bp95/$bp99\`, integrity_failures=\`$binteg\` (\`$brun\`)."
  fi
  if [ -n "$best_wall_line" ]; then
    IFS=',' read -r ww wcm _ wtp wwall _ wp95 wp99 _ _ winteg _ _ _ _ _ _ wrun <<<"$best_wall_line"
    echo "- Best wall-time point: workers=\`$ww\`, max-concurrent-jobs=\`$wcm\`, wall=\`$wwall\`, throughput=\`$wtp\`, p95/p99=\`$wp95/$wp99\`, integrity_failures=\`$winteg\` (\`$wrun\`)."
  fi
} >"$summary_md"

echo
echo "workers sweep complete"
echo "session:  $session_dir"
echo "summary:  $summary_md"
echo "csv:      $summary_csv"
