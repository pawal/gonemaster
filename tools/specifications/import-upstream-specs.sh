#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
UPSTREAM_REPO=${1:-"$REPO_ROOT/zonemaster"}

SOURCE_BASE="$UPSTREAM_REPO/docs/public/specifications/tests"
DEST_TESTS="$REPO_ROOT/docs/specifications/upstream/tests"
DEST_COMMON="$REPO_ROOT/docs/specifications/upstream/common"

COMMON_FILES=(
  "ArgumentsForTestCaseMessages.md"
  "DNSQueryAndResponseDefaults.md"
  "ImplementedTestCases.md"
  "MasterTestPlan.md"
  "Methods.md"
  "MethodsV2.md"
  "RequirementsAndNormalizationOfDomainNames.md"
  "SeverityLevelDefinitions.md"
  "TestMessages.md"
)

if [ ! -d "$UPSTREAM_REPO/.git" ]; then
  echo "error: upstream repo not found or not a git repo: $UPSTREAM_REPO" >&2
  exit 1
fi

if [ ! -d "$SOURCE_BASE" ]; then
  echo "error: upstream source directory missing: $SOURCE_BASE" >&2
  exit 1
fi

mkdir -p "$DEST_TESTS" "$DEST_COMMON"

write_with_provenance() {
  local src_file=$1
  local dst_file=$2
  local src_rel commit date

  src_rel=${src_file#"$UPSTREAM_REPO/"}
  commit=$(git -C "$UPSTREAM_REPO" log -1 --format='%H' -- "$src_rel" 2>/dev/null || true)
  date=$(git -C "$UPSTREAM_REPO" log -1 --format='%cI' -- "$src_rel" 2>/dev/null || true)

  if [ -z "$commit" ]; then
    commit=$(git -C "$UPSTREAM_REPO" rev-parse HEAD)
  fi
  if [ -z "$date" ]; then
    date=$(git -C "$UPSTREAM_REPO" show -s --format='%cI' "$commit")
  fi

  {
    echo "<!--"
    echo "Upstream-Source: $src_rel"
    echo "Upstream-Commit: $commit"
    echo "Upstream-Date: $date"
    echo "-->"
    echo
    cat "$src_file"
  } > "$dst_file"
}

test_files=0
for module_dir in "$SOURCE_BASE"/*-TP; do
  [ -d "$module_dir" ] || continue
  module_name=$(basename "$module_dir")
  target_dir="$DEST_TESTS/$module_name"

  rm -rf "$target_dir"
  cp -R "$module_dir" "$target_dir"

  while IFS= read -r -d '' target_md; do
    rel_in_module=${target_md#"$target_dir/"}
    source_md="$module_dir/$rel_in_module"
    write_with_provenance "$source_md" "$target_md"
    test_files=$((test_files + 1))
  done < <(find "$target_dir" -type f -name '*.md' -print0)
done

common_files=0
for file_name in "${COMMON_FILES[@]}"; do
  source_file="$SOURCE_BASE/$file_name"
  target_file="$DEST_COMMON/$file_name"

  if [ ! -f "$source_file" ]; then
    echo "error: missing upstream common file: $source_file" >&2
    exit 1
  fi

  write_with_provenance "$source_file" "$target_file"
  common_files=$((common_files + 1))
done

echo "Imported upstream specs: tests=$test_files common=$common_files"
