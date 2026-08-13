#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Run interleaved server performance matrix for multiple variants.

Usage:
  run_tracks_matrix.sh --variants FILE --domains FILE [options]

Required:
  --variants FILE              Variant file: "name ref [profile]" per line
  --domains FILE               Domain list (one domain per line)

The optional profile column overrides --profile for that variant; use "-" to
keep the global one. Variants sharing a ref share one worktree and binary.

Options:
  --out-dir DIR                Output directory (default: perf-runs/server/tracks-<utc>)
  --warmups N                  Warmup runs per variant (default: 1)
  --repeats N                  Measured runs per variant (default: 7)
  --workers N                  Server workers (default: 8)
  --max-concurrent-jobs N      Engine limiter (default: 8)
  --port N                     Listen port (default: 18080)
  --profile FILE               Profile path passed to server (optional)
  --inter-run-sleep N          Cool-down between runs in seconds (default: 0)
  --min-level LEVEL            Server min-level (default: INFO)
  --batch-poll-seconds N       Batch polling interval (default: 2)
  --sample-seconds N           Sampling interval (default: 1)
  --batch-timeout-seconds N    Timeout per run (default: 5400)
  --worktree-root DIR          Worktree root (default: /tmp/gonemaster-perf-worktrees-<utc>)
  --go-cache DIR               GOCACHE (default: /tmp/gocache)
  --keep-worktrees             Keep temporary worktrees after run
  --help                       Show this help
EOF
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

json_quote() {
  jq -Rsa . <<<"${1:-}"
}

wait_health() {
  local base_url="$1"
  local timeout_s="$2"
  local start now
  start="$(date +%s)"
  while true; do
    if curl -fsS "${base_url}/api/v1/healthz" >/dev/null 2>&1; then
      return 0
    fi
    now="$(date +%s)"
    if [ $((now - start)) -ge "$timeout_s" ]; then
      return 1
    fi
    sleep 0.4
  done
}

sample_stats_json() {
  local csv="$1"
  awk -F',' '
  NR==1 { next }
  {
    n++
    q+=$2; if ($2>qmax) qmax=$2
    f+=$3; if ($3>fmax) fmax=$3
    c+=$9; if ($9>cmax) cmax=$9
    r+=$10; if ($10>rmax) rmax=$10
  }
  END {
    if (n==0) {
      printf "{\"queue_avg\":0,\"queue_max\":0,\"in_flight_avg\":0,\"in_flight_max\":0,\"cpu_avg\":0,\"cpu_max\":0,\"rss_avg\":0,\"rss_max\":0}"
    } else {
      printf "{\"queue_avg\":%.6f,\"queue_max\":%d,\"in_flight_avg\":%.6f,\"in_flight_max\":%d,\"cpu_avg\":%.6f,\"cpu_max\":%.6f,\"rss_avg\":%.6f,\"rss_max\":%d}",
        q/n, qmax, f/n, fmax, c/n, cmax, r/n, rmax
    }
  }' "$csv"
}

duration_stats_json() {
  local batch_final="$1"
  jq -c '
    def ts: sub("\\..*Z$";"Z") | fromdateiso8601;
    [ .items[]
      | select(.started_at != null and .finished_at != null and .started_at != "" and .finished_at != "")
      | ((.finished_at|ts) - (.started_at|ts)) * 1000
    ] as $d
    | ($d | sort) as $s
    | ($s | length) as $n
    | def pick($p):
        if $n == 0 then 0
        else $s[ (((($n - 1) * $p) | floor) ) ] end;
    {
      count: $n,
      avg: (if $n == 0 then 0 else (($d | add) / $n) end),
      p50: (pick(0.50) | floor),
      p95: (pick(0.95) | floor),
      p99: (pick(0.99) | floor)
    }' "$batch_final"
}

compute_markdown_summary() {
  local report_json="$1"
  local summary_md="$2"
  {
    echo "# Server Perf Comparison"
    echo
    echo "- Generated: \`$(jq -r '.generated_at_utc' "$report_json")\`"
    echo "- Domains file: \`$(jq -r '.domains_file' "$report_json")\` (\`$(jq -r '.domains_count' "$report_json")\` domains, sha256 \`$(jq -r '.domains_sha256' "$report_json")\`)"
    echo "- Warmups per variant: \`$(jq -r '.warmup_runs_per_variant' "$report_json")\`"
    echo "- Measured runs per variant: \`$(jq -r '.measured_runs_per_variant' "$report_json")\`"
    echo "- Cool-down between runs: \`$(jq -r '.inter_run_sleep_seconds' "$report_json")s\`"
    echo "- Server settings: \`workers=$(jq -r '.workers' "$report_json")\`, \`max-concurrent-jobs=$(jq -r '.max_concurrent_jobs' "$report_json")\`"
    echo
    echo "## Variant Provenance"
    echo
    echo "| Variant | Ref | Commit | Binary SHA256 | Profile | Profile SHA256 |"
    echo "| --- | --- | --- | --- | --- | --- |"
    jq -r 'def dash: if (. // "") == "" then "-" else . end;
      .variants[] | "| \(.name) | `\(.ref)` | `\(.commit_sha)` | `\(.binary_sha256)` | `\(.profile|dash)` | `\(.profile_sha256|dash)` |"' "$report_json"
    echo
    echo "## Aggregate Metrics (measured runs only)"
    echo
    echo "| Variant | Median Throughput (jobs/s) | Median Wall (s) | p50/p95/p99 (ms) | Success Rate | Failed Rate | Integrity Failures |"
    echo "| --- | ---: | ---: | --- | ---: | ---: | ---: |"
    jq -r '.aggregate[] | "| \(.variant) | \(.median_throughput_jobs_per_s|tostring) | \(.median_wall_seconds|tostring) | \(.job_duration_p50_ms)/\(.job_duration_p95_ms)/\(.job_duration_p99_ms) | \(.success_rate|tostring) | \(.failed_rate|tostring) | \(.integrity_failures|tostring) |"' "$report_json"
    echo
    echo "## Run-Level Metrics"
    echo
    echo "| Variant | Run | Warmup | Wall (s) | Throughput (jobs/s) | Completed | Succeeded | Failed | Canceled | Expired | p50/p95/p99 (ms) | Queue avg/max | In-flight avg/max | CPU avg/max | RSS avg/max (KB) | Integrity |"
    echo "| --- | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- | --- | --- | --- | --- |"
    jq -r '.runs[] | "| \(.variant) | \(.run_index) | \(.warmup) | \(.wall_seconds|tostring) | \(.throughput_jobs_per_s|tostring) | \(.completed_terminal) | \(.succeeded) | \(.failed) | \(.canceled) | \(.expired) | \(.job_duration_p50_ms)/\(.job_duration_p95_ms)/\(.job_duration_p99_ms) | \(.queue_depth_avg)/\(.queue_depth_max) | \(.in_flight_avg)/\(.in_flight_max) | \(.server_cpu_percent_avg)/\(.server_cpu_percent_max) | \(.server_rss_kb_avg)/\(.server_rss_kb_max) | \(.run_integrity_ok) |"' "$report_json"
  } >"$summary_md"
}

variants_file=""
domains_file=""
out_dir=""
warmups=1
repeats=7
workers=8
max_concurrent_jobs=8
port=18080
profile_file=""
min_level="INFO"
batch_poll_seconds=2
sample_seconds=1
batch_timeout_seconds=5400
inter_run_sleep=0
worktree_root=""
go_cache="/tmp/gocache"
keep_worktrees=0

while [ $# -gt 0 ]; do
  case "$1" in
    --variants) variants_file="$2"; shift 2 ;;
    --domains) domains_file="$2"; shift 2 ;;
    --out-dir) out_dir="$2"; shift 2 ;;
    --warmups) warmups="$2"; shift 2 ;;
    --repeats) repeats="$2"; shift 2 ;;
    --workers) workers="$2"; shift 2 ;;
    --max-concurrent-jobs) max_concurrent_jobs="$2"; shift 2 ;;
    --port) port="$2"; shift 2 ;;
    --profile) profile_file="$2"; shift 2 ;;
    --min-level) min_level="$2"; shift 2 ;;
    --batch-poll-seconds) batch_poll_seconds="$2"; shift 2 ;;
    --sample-seconds) sample_seconds="$2"; shift 2 ;;
    --batch-timeout-seconds) batch_timeout_seconds="$2"; shift 2 ;;
    --inter-run-sleep) inter_run_sleep="$2"; shift 2 ;;
    --worktree-root) worktree_root="$2"; shift 2 ;;
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

if [ -z "$variants_file" ] || [ -z "$domains_file" ]; then
  usage
  exit 1
fi
if [ ! -f "$variants_file" ]; then
  echo "variants file not found: $variants_file" >&2
  exit 1
fi
if [ ! -f "$domains_file" ]; then
  echo "domains file not found: $domains_file" >&2
  exit 1
fi
if [ -n "$profile_file" ] && [ ! -f "$profile_file" ]; then
  echo "profile file not found: $profile_file" >&2
  exit 1
fi

need_cmd git
need_cmd go
need_cmd jq
need_cmd curl
need_cmd awk
need_cmd sed
need_cmd ps
need_cmd sha256sum

repo_root="$(git rev-parse --show-toplevel)"
run_ts="$(date -u +%Y%m%d-%H%M%S)"
if [ -z "$out_dir" ]; then
  out_dir="$repo_root/perf-runs/server/tracks-$run_ts"
fi
if [ -z "$worktree_root" ]; then
  worktree_root="/tmp/gonemaster-perf-worktrees-$run_ts"
fi

mkdir -p "$out_dir" "$out_dir/bin" "$out_dir/runs" "$out_dir/variants" "$worktree_root" "$go_cache"

domains_clean="$out_dir/domains.txt"
awk 'NF { gsub(/\r/,""); print tolower($0) }' "$domains_file" | awk '!seen[$0]++' >"$domains_clean"
domains_count="$(wc -l <"$domains_clean" | awk '{print $1}')"
if [ "$domains_count" -le 0 ]; then
  echo "no domains after normalization: $domains_file" >&2
  exit 1
fi
domains_sha="$(sha256sum "$domains_clean" | awk '{print $1}')"

payload_file="$out_dir/batch_payload.json"
jq -n --rawfile lines "$domains_clean" '{domains: ($lines | split("\n") | map(select(length>0)))}' >"$payload_file"

variant_names=()
variant_refs=()
variant_profiles=()
while IFS= read -r raw; do
  line="${raw%%#*}"
  line="$(echo "$line" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
  [ -n "$line" ] || continue
  set -- $line
  if [ $# -lt 2 ]; then
    echo "invalid variants line (need: name ref [profile]): $raw" >&2
    exit 1
  fi
  name="$1"
  ref="$2"
  variant_profile="${3:-}"
  if [ "$variant_profile" = "-" ]; then
    variant_profile=""
  fi
  if ! [[ "$name" =~ ^[A-Za-z0-9._-]+$ ]]; then
    echo "invalid variant name: $name" >&2
    exit 1
  fi
  if [ -n "$variant_profile" ] && [ ! -f "$variant_profile" ]; then
    echo "variant profile not found for $name: $variant_profile" >&2
    exit 1
  fi
  variant_names+=("$name")
  variant_refs+=("$ref")
  variant_profiles+=("$variant_profile")
done <"$variants_file"

variant_count="${#variant_names[@]}"
if [ "$variant_count" -lt 1 ]; then
  echo "need at least 1 variant" >&2
  exit 1
fi

worktrees_created=()
cleanup() {
  if [ "$keep_worktrees" -eq 1 ]; then
    return
  fi
  for wt in "${worktrees_created[@]:-}"; do
    if [ -d "$wt" ]; then
      git -C "$repo_root" worktree remove "$wt" --force >/dev/null 2>&1 || true
    fi
  done
}
trap cleanup EXIT

# Variants sharing a ref share one worktree and one binary. A knob-only
# sweep therefore builds once, and every variant in it is provably the same
# binary rather than two builds asserted to be equal.
variant_worktrees=()
variant_owners=()
build_names=()
build_refs=()
for i in "${!variant_names[@]}"; do
  name="${variant_names[$i]}"
  ref="${variant_refs[$i]}"
  owner=""
  for j in "${!build_names[@]}"; do
    if [ "${build_refs[$j]}" = "$ref" ]; then
      owner="${build_names[$j]}"
      break
    fi
  done
  if [ -z "$owner" ]; then
    owner="$name"
    wt="$worktree_root/$name"
    git -C "$repo_root" worktree add --detach "$wt" "$ref" >/dev/null
    worktrees_created+=("$wt")
    build_names+=("$name")
    build_refs+=("$ref")
  else
    wt="$worktree_root/$owner"
  fi
  variant_worktrees+=("$wt")
  variant_owners+=("$owner")
done

echo "Building ${#build_names[@]} binaries for ${variant_count} variants in parallel..."
build_pids=()
for i in "${!variant_names[@]}"; do
  [ "${variant_owners[$i]}" = "${variant_names[$i]}" ] || continue
  (
    set -euo pipefail
    name="${variant_names[$i]}"
    wt="${variant_worktrees[$i]}"
    commit_sha="$(git -C "$wt" rev-parse HEAD)"
    bin_path="$out_dir/bin/gonemaster-server-$name"
    GOCACHE="$go_cache" go -C "$wt" build -o "$bin_path" ./cmd/gonemaster-server
    echo "$commit_sha" >"$out_dir/bin/$name.commit"
  ) &
  build_pids+=("$!")
done
for p in "${build_pids[@]}"; do
  wait "$p"
done

for i in "${!variant_names[@]}"; do
  name="${variant_names[$i]}"
  ref="${variant_refs[$i]}"
  wt="${variant_worktrees[$i]}"
  owner="${variant_owners[$i]}"
  bin_path="$out_dir/bin/gonemaster-server-$owner"
  commit_sha="$(cat "$out_dir/bin/$owner.commit")"
  bin_sha="$(sha256sum "$bin_path" | awk '{print $1}')"
  variant_profile="${variant_profiles[$i]}"
  if [ -z "$variant_profile" ]; then
    variant_profile="$profile_file"
  fi
  profile_sha=""
  if [ -n "$variant_profile" ]; then
    profile_sha="$(sha256sum "$variant_profile" | awk '{print $1}')"
  fi
  jq -n \
    --arg name "$name" \
    --arg ref "$ref" \
    --arg worktree "$wt" \
    --arg commit_sha "$commit_sha" \
    --arg binary "$bin_path" \
    --arg binary_sha256 "$bin_sha" \
    --arg profile "$variant_profile" \
    --arg profile_sha256 "$profile_sha" \
    '{name:$name,ref:$ref,worktree:$worktree,commit_sha:$commit_sha,binary:$binary,binary_sha256:$binary_sha256,profile:$profile,profile_sha256:$profile_sha256}' \
    >"$out_dir/variants/$name.json"
done

jq -s '.' "$out_dir"/variants/*.json >"$out_dir/variants.json"

run_order="$out_dir/run_order.tsv"
{
  echo -e "variant\trun_index\twarmup"
  if [ "$warmups" -gt 0 ]; then
    for ((i=1; i<=warmups; i++)); do
      for name in "${variant_names[@]}"; do
        echo -e "${name}\t${i}\ttrue"
      done
    done
  fi
  if [ "$repeats" -gt 0 ]; then
    for ((i=1; i<=repeats; i++)); do
      for name in "${variant_names[@]}"; do
        echo -e "${name}\t${i}\tfalse"
      done
    done
  fi
} >"$run_order"

runs_csv="$out_dir/runs.csv"
echo "variant,run_index,warmup,batch_id,started_at,finished_at,wall_seconds,throughput_jobs_per_s,completed_terminal,succeeded,failed,canceled,expired,job_duration_count,job_duration_p50_ms,job_duration_p95_ms,job_duration_p99_ms,queue_depth_avg,queue_depth_max,in_flight_avg,in_flight_max,server_cpu_percent_avg,server_cpu_percent_max,server_rss_kb_avg,server_rss_kb_max,run_integrity_ok" >"$runs_csv"

run_total=$((variant_count * (warmups + repeats)))
run_num=0

while IFS=$'\t' read -r variant run_index warmup; do
  [ "$variant" != "variant" ] || continue
  run_num=$((run_num + 1))
  echo "[$run_num/$run_total] variant=$variant run=$run_index warmup=$warmup"

  run_tag="$variant-run-$(printf '%02d' "$run_index")"
  if [ "$warmup" = "true" ]; then
    run_tag="$variant-warmup-$(printf '%02d' "$run_index")"
  fi
  run_dir="$out_dir/runs/$run_tag"
  mkdir -p "$run_dir"

  variant_meta="$out_dir/variants/$variant.json"
  if [ ! -f "$variant_meta" ]; then
    echo "missing variant metadata for $variant" >&2
    exit 1
  fi
  binary_path="$(jq -r '.binary' "$variant_meta")"
  if [ ! -x "$binary_path" ]; then
    echo "binary not executable: $binary_path" >&2
    exit 1
  fi

  run_profile="$(jq -r '.profile // ""' "$variant_meta")"
  base_url="http://127.0.0.1:$port"
  server_cmd=("$binary_path" "--listen" "127.0.0.1:$port" "--workers" "$workers" "--max-concurrent-jobs" "$max_concurrent_jobs" "--min-level" "$min_level")
  if [ -n "$run_profile" ]; then
    server_cmd+=("--profile" "$run_profile")
  fi
  "${server_cmd[@]}" >"$run_dir/server.stdout.log" 2>"$run_dir/server.stderr.log" &
  server_pid="$!"
  echo "$server_pid" >"$run_dir/server.pid"

  if ! wait_health "$base_url" 90; then
    echo "health check failed for $run_tag" >&2
    kill -TERM "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" >/dev/null 2>&1 || true
    exit 1
  fi

  start_ns="$(date +%s%N)"
  started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  curl -fsS -X POST \
    -H "Content-Type: application/json" \
    --data @"$payload_file" \
    "$base_url/api/v1/jobs/batch" >"$run_dir/batch_submit.json"

  batch_id="$(jq -r '.batch_id // empty' "$run_dir/batch_submit.json")"
  if [ -z "$batch_id" ]; then
    echo "batch submission failed for $run_tag" >&2
    kill -TERM "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" >/dev/null 2>&1 || true
    exit 1
  fi

  samples_csv="$run_dir/samples.csv"
  echo "timestamp_utc,queue_depth,in_flight_jobs,active_workers,completed_total,duration_p50_ms,duration_p90_ms,duration_p99_ms,server_cpu_percent,server_rss_kb" >"$samples_csv"
  sampler_stop="$run_dir/.sampler.stop"
  rm -f "$sampler_stop"

  (
    while [ ! -f "$sampler_stop" ]; do
      ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
      metrics="$(curl -fsS "$base_url/api/v1/metrics?include=health,jobs,quality" 2>/dev/null || true)"
      if [ -n "$metrics" ] && jq -e . >/dev/null 2>&1 <<<"$metrics"; then
        queue_depth="$(jq -r '.health.queue_depth // 0' <<<"$metrics")"
        in_flight_jobs="$(jq -r '.health.in_flight_jobs // 0' <<<"$metrics")"
        active_workers="$(jq -r '.health.active_workers // 0' <<<"$metrics")"
        completed_total="$(jq -r '.jobs.completed_total // 0' <<<"$metrics")"
        duration_p50="$(jq -r '.quality.job_duration_ms.percentiles.p50 // 0' <<<"$metrics")"
        duration_p90="$(jq -r '.quality.job_duration_ms.percentiles.p90 // 0' <<<"$metrics")"
        duration_p99="$(jq -r '.quality.job_duration_ms.percentiles.p99 // 0' <<<"$metrics")"
      else
        queue_depth=0
        in_flight_jobs=0
        active_workers=0
        completed_total=0
        duration_p50=0
        duration_p90=0
        duration_p99=0
      fi

      ps_line="$(ps -p "$server_pid" -o %cpu= -o rss= 2>/dev/null || true)"
      if [ -n "$ps_line" ]; then
        cpu="$(awk '{print $1}' <<<"$ps_line")"
        rss="$(awk '{print $2}' <<<"$ps_line")"
      else
        cpu=0
        rss=0
      fi
      echo "$ts,$queue_depth,$in_flight_jobs,$active_workers,$completed_total,$duration_p50,$duration_p90,$duration_p99,$cpu,$rss" >>"$samples_csv"
      sleep "$sample_seconds"
    done
  ) &
  sampler_pid="$!"

  deadline=$(( $(date +%s) + batch_timeout_seconds ))
  run_timed_out=0
  while true; do
    if ! curl -fsS "$base_url/api/v1/batches/$batch_id" >"$run_dir/batch_latest.json"; then
      sleep "$batch_poll_seconds"
      continue
    fi
    total="$(jq -r '.total // 0' "$run_dir/batch_latest.json")"
    succeeded="$(jq -r '.status_counts.succeeded // 0' "$run_dir/batch_latest.json")"
    failed="$(jq -r '.status_counts.failed // 0' "$run_dir/batch_latest.json")"
    canceled="$(jq -r '.status_counts.canceled // 0' "$run_dir/batch_latest.json")"
    expired="$(jq -r '.status_counts.expired // 0' "$run_dir/batch_latest.json")"
    completed=$((succeeded + failed + canceled + expired))
    if [ "$total" -gt 0 ] && [ "$completed" -ge "$total" ]; then
      cp "$run_dir/batch_latest.json" "$run_dir/batch_final.json"
      break
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
      run_timed_out=1
      cp "$run_dir/batch_latest.json" "$run_dir/batch_final.json"
      break
    fi
    sleep "$batch_poll_seconds"
  done

  finished_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  end_ns="$(date +%s%N)"
  wall_seconds="$(awk -v s="$start_ns" -v e="$end_ns" 'BEGIN { printf "%.6f", (e - s) / 1000000000 }')"

  touch "$sampler_stop"
  wait "$sampler_pid" >/dev/null 2>&1 || true

  curl -fsS "$base_url/api/v1/metrics?include=health,jobs,quality" >"$run_dir/metrics_final.json" 2>/dev/null || echo '{}' >"$run_dir/metrics_final.json"

  sample_stats="$(sample_stats_json "$samples_csv")"
  duration_stats="$(duration_stats_json "$run_dir/batch_final.json")"
  status_counts="$(jq '.status_counts // {}' "$run_dir/batch_final.json")"

  total="$(jq -r '.total // 0' "$run_dir/batch_final.json")"
  succeeded="$(jq -r '.status_counts.succeeded // 0' "$run_dir/batch_final.json")"
  failed="$(jq -r '.status_counts.failed // 0' "$run_dir/batch_final.json")"
  canceled="$(jq -r '.status_counts.canceled // 0' "$run_dir/batch_final.json")"
  expired="$(jq -r '.status_counts.expired // 0' "$run_dir/batch_final.json")"
  completed=$((succeeded + failed + canceled + expired))
  throughput="$(awk -v c="$completed" -v w="$wall_seconds" 'BEGIN { if (w <= 0) print 0; else printf "%.9f", c / w }')"

  run_integrity_ok=true
  integrity_notes=()
  if [ "$total" -ne "$domains_count" ]; then
    run_integrity_ok=false
    integrity_notes+=("total(${total})!=domains(${domains_count})")
  fi
  if [ "$completed" -lt "$total" ]; then
    run_integrity_ok=false
    integrity_notes+=("completed(${completed})<total(${total})")
  fi
  if [ "$run_timed_out" -eq 1 ]; then
    run_integrity_ok=false
    integrity_notes+=("timeout")
  fi

  integrity_json="$(printf '%s\n' "${integrity_notes[@]:-}" | jq -Rsc 'split("\n") | map(select(length>0))')"

  jq -n \
    --arg variant "$variant" \
    --argjson run_index "$run_index" \
    --argjson warmup "$warmup" \
    --arg batch_id "$batch_id" \
    --arg started_at "$started_at" \
    --arg finished_at "$finished_at" \
    --argjson wall_seconds "$wall_seconds" \
    --argjson domains_total "$domains_count" \
    --argjson status_counts "$status_counts" \
    --argjson completed_terminal "$completed" \
    --argjson succeeded "$succeeded" \
    --argjson failed "$failed" \
    --argjson canceled "$canceled" \
    --argjson expired "$expired" \
    --argjson throughput_jobs_per_s "$throughput" \
    --argjson duration "$duration_stats" \
    --argjson sample "$sample_stats" \
    --argjson run_integrity_ok "$run_integrity_ok" \
    --argjson integrity_notes "$integrity_json" \
    '{
      variant: $variant,
      run_index: $run_index,
      warmup: $warmup,
      batch_id: $batch_id,
      started_at: $started_at,
      finished_at: $finished_at,
      wall_seconds: $wall_seconds,
      domains_total: $domains_total,
      status_counts: $status_counts,
      completed_terminal: $completed_terminal,
      succeeded: $succeeded,
      failed: $failed,
      canceled: $canceled,
      expired: $expired,
      throughput_jobs_per_s: $throughput_jobs_per_s,
      job_duration_count: ($duration.count // 0),
      job_duration_avg_ms: ($duration.avg // 0),
      job_duration_p50_ms: ($duration.p50 // 0),
      job_duration_p95_ms: ($duration.p95 // 0),
      job_duration_p99_ms: ($duration.p99 // 0),
      queue_depth_avg: ($sample.queue_avg // 0),
      queue_depth_max: ($sample.queue_max // 0),
      in_flight_avg: ($sample.in_flight_avg // 0),
      in_flight_max: ($sample.in_flight_max // 0),
      server_cpu_percent_avg: ($sample.cpu_avg // 0),
      server_cpu_percent_max: ($sample.cpu_max // 0),
      server_rss_kb_avg: ($sample.rss_avg // 0),
      server_rss_kb_max: ($sample.rss_max // 0),
      run_integrity_ok: $run_integrity_ok,
      integrity_notes: $integrity_notes
    }' >"$run_dir/summary.json"

  jq -r '[.variant,.run_index,.warmup,.batch_id,.started_at,.finished_at,.wall_seconds,.throughput_jobs_per_s,.completed_terminal,.succeeded,.failed,.canceled,.expired,.job_duration_count,.job_duration_p50_ms,.job_duration_p95_ms,.job_duration_p99_ms,.queue_depth_avg,.queue_depth_max,.in_flight_avg,.in_flight_max,.server_cpu_percent_avg,.server_cpu_percent_max,.server_rss_kb_avg,.server_rss_kb_max,.run_integrity_ok] | @csv' \
    "$run_dir/summary.json" >>"$runs_csv"

  kill -TERM "$server_pid" >/dev/null 2>&1 || true
  for _ in $(seq 1 30); do
    if ! kill -0 "$server_pid" >/dev/null 2>&1; then
      break
    fi
    sleep 0.2
  done
  kill -KILL "$server_pid" >/dev/null 2>&1 || true
  wait "$server_pid" >/dev/null 2>&1 || true

  if [ "$inter_run_sleep" -gt 0 ] && [ "$run_num" -lt "$run_total" ]; then
    echo "  cool-down ${inter_run_sleep}s"
    sleep "$inter_run_sleep"
  fi
done <"$run_order"

report_json="$out_dir/report.json"
jq -s \
  --arg generated_at_utc "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg domains_file "$domains_clean" \
  --arg domains_sha256 "$domains_sha" \
  --argjson domains_count "$domains_count" \
  --argjson warmups "$warmups" \
  --argjson repeats "$repeats" \
  --argjson inter_run_sleep_seconds "$inter_run_sleep" \
  --argjson workers "$workers" \
  --argjson max_concurrent_jobs "$max_concurrent_jobs" \
  --argjson variants "$(cat "$out_dir/variants.json")" \
  '
  def median(a):
    (a | sort) as $s
    | ($s | length) as $n
    | if $n == 0 then 0
      elif ($n % 2) == 1 then $s[$n / 2]
      else (($s[$n/2 - 1] + $s[$n/2]) / 2)
      end;
  def q1(a):
    (a | sort) as $s
    | ($s | length) as $n
    | if $n == 0 then 0 else $s[((($n - 1) * 0.25) | floor)] end;
  def q3(a):
    (a | sort) as $s
    | ($s | length) as $n
    | if $n == 0 then 0 else $s[((($n - 1) * 0.75) | floor)] end;
  . as $runs
  | {
    generated_at_utc: $generated_at_utc,
    domains_file: $domains_file,
    domains_sha256: $domains_sha256,
    domains_count: $domains_count,
    warmup_runs_per_variant: $warmups,
    measured_runs_per_variant: $repeats,
    inter_run_sleep_seconds: $inter_run_sleep_seconds,
    workers: $workers,
    max_concurrent_jobs: $max_concurrent_jobs,
    variants: $variants,
    runs: $runs,
    aggregate: (
      [ $runs[] | select(.warmup == false) | .variant ] | unique | map(
        . as $variant |
        ([ $runs[] | select(.variant == $variant and .warmup == false) ]) as $r |
        ($r | map(.completed_terminal) | add // 0) as $completed_sum |
        ($r | map(.succeeded) | add // 0) as $succeeded_sum |
        ($r | map(.failed) | add // 0) as $failed_sum |
        {
          variant: $variant,
          runs: ($r | length),
          median_wall_seconds: median($r | map(.wall_seconds)),
          iqr_wall_seconds_q1: q1($r | map(.wall_seconds)),
          iqr_wall_seconds_q3: q3($r | map(.wall_seconds)),
          median_throughput_jobs_per_s: median($r | map(.throughput_jobs_per_s)),
          iqr_throughput_q1: q1($r | map(.throughput_jobs_per_s)),
          iqr_throughput_q3: q3($r | map(.throughput_jobs_per_s)),
          total_jobs: $completed_sum,
          success_rate: (if $completed_sum == 0 then 0 else ($succeeded_sum / $completed_sum) end),
          failed_rate: (if $completed_sum == 0 then 0 else ($failed_sum / $completed_sum) end),
          job_duration_p50_ms: median($r | map(.job_duration_p50_ms)),
          job_duration_p95_ms: median($r | map(.job_duration_p95_ms)),
          job_duration_p99_ms: median($r | map(.job_duration_p99_ms)),
          median_queue_depth_max: median($r | map(.queue_depth_max)),
          median_in_flight_max: median($r | map(.in_flight_max)),
          median_cpu_percent_max: median($r | map(.server_cpu_percent_max)),
          median_rss_kb_max: median($r | map(.server_rss_kb_max)),
          integrity_failures: ($r | map(select(.run_integrity_ok != true)) | length)
        }
      )
    )
  }' "$out_dir"/runs/*/summary.json >"$report_json"

compute_markdown_summary "$report_json" "$out_dir/SUMMARY.md"

echo
echo "Completed matrix run."
echo "Output: $out_dir"
echo "Report: $out_dir/report.json"
echo "Summary: $out_dir/SUMMARY.md"
