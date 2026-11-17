#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 3 || $# -gt 4 || ! $2 =~ ^[0-9]+$ || ! $3 =~ ^[1-9][0-9]*$ ]] || (( $2 >= $3 )); then
  echo "usage: $0 PACKAGE SHARD_INDEX SHARD_COUNT [--list]" >&2
  exit 2
fi
if [[ $# -eq 4 && $4 != --list ]]; then
  echo "usage: $0 PACKAGE SHARD_INDEX SHARD_COUNT [--list]" >&2
  exit 2
fi

package=$1
shard=$2
count=$3
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

go test -race -short -vet=off -list . "$package" > "$tmp/names"
python3 tools/ci/race-balance.py plan \
  --package "$package" --tests "$tmp/names" \
  --profile "${RACE_PROFILE_FILE:-tools/ci/race-test-durations.json}" \
  --index "$shard" --count "$count" > "$tmp/selected"
mapfile -t selected < "$tmp/selected"
if (( ${#selected[@]} == 0 )); then
  echo "shard $shard/$count has no tests" >&2
  exit 1
fi

if [[ ${4:-} == --list ]]; then
  printf '%s\n' "${selected[@]}"
  exit 0
fi

printf 'Running %s race shard %d/%d (%d tests)\n' \
  "$package" "$shard" "$count" "${#selected[@]}"
pattern="^($(IFS='|'; echo "${selected[*]}"))$"
events=${RACE_EVENTS_FILE:-"$tmp/events.jsonl"}
fragment=${RACE_FRAGMENT_FILE:-"$tmp/durations.json"}

# Preserve real test failures with pipefail; record elapsed time for passing top-level tests.
go test -race -short -vet=off -timeout 20m -json -run "$pattern" "$package" | tee "$events"
python3 tools/ci/race-balance.py extract \
  --package "$package" --input "$events" --output "$fragment"
